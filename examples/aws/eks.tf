provider "aws" {
  region = var.region
}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name                    = var.vpc_name
  cidr                    = var.vpc_cidr
  azs                     = var.availability_zones
  private_subnets         = var.private_cidr_ranges
  public_subnets          = var.public_cidr_ranges
  enable_nat_gateway      = true
  single_nat_gateway      = true
  enable_dns_hostnames    = true
  map_public_ip_on_launch = true

  tags = {
    App = "kubeip"
    Env = "demo"
  }
  public_subnet_tags = {
    public      = "true"
    environment = "demo"
  }
  private_subnet_tags = {
    public      = "false"
    environment = "demo"
  }
}

module "eks" {
  source = "terraform-aws-modules/eks/aws"

  cluster_name    = var.cluster_name
  cluster_version = var.kubernetes_version

  cluster_endpoint_public_access = true

  vpc_id     = module.vpc.vpc_id
  subnet_ids = concat(module.vpc.private_subnets, module.vpc.public_subnets)

  eks_managed_node_groups = {
    eks_nodes_public = {
      desired_size = 3
      max_size     = 5
      min_size     = 1

      instance_types = ["t3a.small", "t3a.medium"]
      capacity_type  = "SPOT"

      labels = {
        nodegroup = "public"
        kubeip    = "use"
      }

      tags = {
        Name        = "public-node-group"
        environment = "demo"
        public      = "true"
        kubeip      = "use"
      }

      subnet_ids = module.vpc.public_subnets
    }

    eks_nodes_private = {
      desired_size = 1
      max_size     = 5
      min_size     = 1

      instance_types = ["t3a.small", "t3a.medium"]
      capacity_type  = "SPOT"

      labels = {
        nodegroup = "private"
        kubeip    = "ignore"
      }

      tags = {
        Name        = "private-node-group"
        environment = "demo"
      }

      subnet_ids = module.vpc.private_subnets
    }
  }
}

resource "aws_iam_policy" "kubeip-policy" {
  name        = "kubeip-policy"
  description = "KubeIP required permissions"

  policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [
      {
        Action = [
          "ec2:AssociateAddress",
          "ec2:DisassociateAddress",
          "ec2:DescribeInstances",
          "ec2:DescribeAddresses"
        ]
        Effect   = "Allow"
        Resource = "*"
      },
    ]
  })
}

module "kubeip_eks_role" {
  source    = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  role_name = "kubeip-eks-role"

  role_policy_arns = {
    "kubeip-policy" = aws_iam_policy.kubeip-policy.arn
  }

  oidc_providers = {
    main = {
      provider_arn               = module.eks.oidc_provider_arn
      namespace_service_accounts = ["kube-system:kubeip-service-account"]
    }
  }
}

# 3 elastic IPs in the same region
resource "aws_eip" "kubeip" {
  // default EIP limit is 5 (make sure to increase it if you need more)
  count = 5

  tags = {
    Name        = "kubeip-${count.index}"
    environment = "demo"
    kubeip      = "reserved"
  }
}

data "aws_eks_cluster_auth" "kubeip_cluster_auth" {
  name = module.eks.cluster_name
}

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  token                  = data.aws_eks_cluster_auth.kubeip_cluster_auth.token
}

resource "kubernetes_service_account" "kubeip_service_account" {
  metadata {
    name        = "kubeip-service-account"
    namespace   = "kube-system"
    annotations = {
      "eks.amazonaws.com/role-arn" = module.kubeip_eks_role.iam_role_arn
    }
  }
  depends_on = [module.eks]
}

# Create cluster role with get node permission
resource "kubernetes_cluster_role" "kubeip_cluster_role" {
  metadata {
    name = "kubeip-cluster-role"
  }
  rule {
    api_groups = ["*"]
    resources  = ["nodes"]
    verbs      = ["get"]
  }
  rule {
    api_groups = ["coordination.k8s.io"]
    resources  = ["leases"]
    verbs      = ["create", "delete", "get"]
  }
  depends_on = [
    kubernetes_service_account.kubeip_service_account,
    module.eks
  ]
}

# Bind cluster role to kubeip service account
resource "kubernetes_cluster_role_binding" "kubeip_cluster_role_binding" {
  metadata {
    name = "kubeip-cluster-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role.kubeip_cluster_role.metadata[0].name
  }
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.kubeip_service_account.metadata[0].name
    namespace = kubernetes_service_account.kubeip_service_account.metadata[0].namespace
  }
  depends_on = [
    kubernetes_service_account.kubeip_service_account,
    kubernetes_cluster_role.kubeip_cluster_role
  ]
}


# Deploy KubeIP DaemonSet
resource "kubernetes_daemonset" "kubeip_daemonset" {
  metadata {
    name      = "kubeip-agent"
    namespace = "kube-system"
    labels    = {
      app = "kubeip"
    }
  }
  spec {
    selector {
      match_labels = {
        app = "kubeip"
      }
    }
    strategy {
      type = "RollingUpdate"
      rolling_update {
        max_unavailable = 1
      }
    }
    template {
      metadata {
        labels = {
          app = "kubeip"
        }
      }
      spec {
        service_account_name             = "kubeip-service-account"
        termination_grace_period_seconds = 30
        priority_class_name              = "system-node-critical"
        toleration {
          effect   = "NoSchedule"
          operator = "Exists"
        }
        toleration {
          effect   = "NoExecute"
          operator = "Exists"
        }
        container {
          name  = "kubeip-agent"
          image = "doitintl/kubeip-agent:${var.kubeip_version}"
          env {
            name = "NODE_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }
          env {
            name  = "FILTER"
            value = "Name=tag:kubeip,Values=reserved;Name=tag:environment,Values=demo"
          }
          env {
            name  = "LOG_LEVEL"
            value = "debug"
          }
          resources {
            requests = {
              cpu    = "10m"
              memory = "32Mi"
            }
          }
        }
        node_selector = {
          nodegroup = "public"
          kubeip    = "use"
        }
      }
    }
  }
  depends_on = [kubernetes_service_account.kubeip_service_account]
}
