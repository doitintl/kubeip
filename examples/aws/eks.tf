provider "aws" {
  region = var.region
}

module "vpc" {
  source = "terraform-aws-modules/vpc/aws"

  name                 = var.vpc_name
  cidr                 = "10.0.0.0/16"
  azs                  = ["us-west-2a", "us-west-2b", "us-west-2c"]
  private_subnets      = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  public_subnets       = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true

  tags = {
    App = "kubeip"
    Env = "demo"
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
      desired_size = 1
      max_size     = 5
      min_size     = 1

      instance_types = ["t4g.micro", "t4g.small"]
      capacity_type  = "SPOT"
      platform       = "bottlerocket"

      taints = [
        {
          key    = "kubeip"
          value  = "use"
          effect = "NO_SCHEDULE"
        }
      ]

      labels = {
        nodegroup = "public"
        kubeip    = "use"
      }

      tags = {
        Environment = "demo"
        Name        = "public-node-group"
        public      = "true"
        kubeip      = "use"
      }

      subnet_ids = module.vpc.public_subnets
    }

    eks_nodes_private = {
      desired_size = 1
      max_size     = 5
      min_size     = 1

      instance_types = ["t4g.micro", "t4g.small"]
      capacity_type  = "SPOT"
      platform       = "bottlerocket"

      labels = {
        nodegroup = "private"
        kubeip    = "ignore"
      }

      tags = {
        Environment = "demo"
        Name        = "private-node-group"
      }

      subnet_ids = module.vpc.private_subnets
    }
  }

  # aws-auth configmap
  manage_aws_auth_configmap = true

  tags = {
    App         = "kubeip"
    Environment = "demo"
  }
}

resource "aws_iam_policy" "kubeip-policy" {
  name        = "kubeip-policy"
  description = "KubeIP policy"

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
      namespace_service_accounts = ["kube-system:kubeip-sa"]
    }
  }
}

# 5 elastic IPs in the same region
resource "aws_eip" "kubeip" {
  count = 5

  tags = {
    Name   = "kubeip-${count.index}"
    kubeip = "reserved"
  }
}
