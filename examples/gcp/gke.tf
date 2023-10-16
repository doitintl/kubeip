# Save state to local file
terraform {
  backend "local" {
    path = "terraform.tfstate"
  }
}

# Set the provider and credentials
provider "google" {
  project = var.project_id
  region  = var.region
}

# Create custom IAM Role
resource "google_project_iam_custom_role" "kubeip_role" {
  role_id     = "kubeip_role"
  title       = "KubeIP Role"
  description = "KubeIP Role"
  stage       = "GA"
  permissions = [
    "compute.instances.addAccessConfig",
    "compute.instances.deleteAccessConfig",
    "compute.instances.get",
    "compute.addresses.list",
    "compute.zoneOperations.get",
    "compute.projects.get"
  ]
}

# Create custom IAM service account
resource "google_service_account" "kubeip_service_account" {
  account_id   = "kubeip-service-account"
  display_name = "KubeIP Service Account"
}

# Bind custom IAM Role to kubeip IAM service account
resource "google_project_iam_member" "kubeip_role_binding" {
  role    = google_project_iam_custom_role.kubeip_role.id
  member  = "serviceAccount:${google_service_account.kubeip_service_account.email}"
  project = var.project_id
}

# Bind workload identity to kubeip IAM service account
resource "google_service_account_iam_member" "kubeip_workload_identity_binding" {
  service_account_id = google_service_account.kubeip_service_account.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "serviceAccount:${var.project_id}.svc.id.goog[kube-system/kubeip-service-account]"
}

# Create a VPC network
resource "google_compute_network" "vpc" {
  name                    = var.vpc_name
  auto_create_subnetworks = false
}

# Create a public subnet
resource "google_compute_subnetwork" "kubeip_subnet" {
  name          = "kubeip-subnet"
  network       = google_compute_network.vpc.id
  region        = var.region
  ip_cidr_range = "10.0.1.0/24"
}

# Create GKE cluster
resource "google_container_cluster" "kubeip_cluster" {
  name     = var.cluster_name
  location = var.region

  initial_node_count       = 1
  remove_default_node_pool = true

  network    = google_compute_network.vpc.id
  subnetwork = google_compute_subnetwork.kubeip_subnet.id

  # Enable Workload Identity
  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }
}

# Create node pools
resource "google_container_node_pool" "public_node_pool" {
  name     = "public-node-pool"
  location = google_container_cluster.kubeip_cluster.location
  cluster  = google_container_cluster.kubeip_cluster.name
  autoscaling {
    total_min_node_count = 1
    total_max_node_count = 5
    location_policy      = "ANY"
  }
  node_config {
    machine_type = var.machine_type
    spot         = true
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
    metadata = {
      disable-legacy-endpoints = "true"
    }
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
    labels = {
      nodegroup = "public"
      kubeip    = "use"
    }
    resource_labels = {
      environment = "demo"
      kubeip      = "use"
      public      = "true"
    }
  }
}

resource "google_container_node_pool" "private_node_pool" {
  name     = "private-node-pool"
  location = google_container_cluster.kubeip_cluster.location
  cluster  = google_container_cluster.kubeip_cluster.name
  autoscaling {
    total_min_node_count = 1
    total_max_node_count = 5
    location_policy      = "ANY"
  }
  node_config {
    machine_type = var.machine_type
    spot         = true
    oauth_scopes = [
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
    metadata = {
      disable-legacy-endpoints = "true"
    }
    workload_metadata_config {
      mode = "GKE_METADATA"
    }
    labels = {
      nodegroup = "private"
      kubeip    = "ignore"
    }
    resource_labels = {
      environment = "demo"
      kubeip      = "ignore"
      public      = "false"
    }
  }
  network_config {
    enable_private_nodes = true
  }
}

# Create static public IP addresses
resource "google_compute_address" "static_ip" {
  provider     = google-beta
  project      = var.project_id
  count        = 5
  name         = "static-ip-${count.index}"
  address_type = "EXTERNAL"
  region       = google_container_cluster.kubeip_cluster.location
  labels       = {
    environment = "demo"
    kubeip      = "reserved"
  }
}

data "google_client_config" "provider" {}

provider "kubernetes" {
  host                   = "https://${google_container_cluster.kubeip_cluster.endpoint}"
  token                  = data.google_client_config.provider.access_token
  cluster_ca_certificate = base64decode(
    google_container_cluster.kubeip_cluster.master_auth[0].cluster_ca_certificate,
  )
}

# Create Kubernetes service account in kube-system namespace
resource "kubernetes_service_account" "kubeip_service_account" {
  metadata {
    name        = "kubeip-service-account"
    namespace   = "kube-system"
    annotations = {
      "iam.gke.io/gcp-service-account" = google_service_account.kubeip_service_account.email
    }
  }
  depends_on = [
    google_service_account.kubeip_service_account,
    google_container_cluster.kubeip_cluster
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
    template {
      metadata {
        labels = {
          app = "kubeip"
        }
      }
      spec {
        service_account_name = "kubeip-service-account"
        container {
          name  = "kubeip-agent"
          image = "doitintl/kubeip-agent"
          env {
            name  = "FILTER"
            value = "label.kubeip=reserved;labels.environment=demo"
          }
          env {
            name  = "LOG_LEVEL"
            value = "debug"
          }
          volume_mount {
            mount_path = "/etc/podinfo"
            name       = "podinfo"
          }
        }
        node_selector = {
          nodegroup = "public"
          kubeip    = "use"
        }
        volume {
          name = "podinfo"
          downward_api {
            items {
              path = "nodeName"
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }
        }
      }
    }
  }
  depends_on = [
    kubernetes_service_account.kubeip_service_account,
    google_container_cluster.kubeip_cluster
  ]
}
