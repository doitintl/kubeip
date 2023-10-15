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

# Bind custom IAM Role to custom IAM service account
resource "google_project_iam_member" "kubeip_role_binding" {
  role    = google_project_iam_custom_role.kubeip_role.id
  member  = "serviceAccount:${google_service_account.kubeip_service_account.email}"
  project = var.project_id
}

# Create a VPC network
resource "google_compute_network" "vpc" {
  name = var.vpc_name
}

# Create a public subnet
resource "google_compute_subnetwork" "public_subnet" {
  name          = "public-subnet"
  network       = google_compute_network.vpc.id
  ip_cidr_range = "10.0.1.0/24"
}

# Create a private subnet
resource "google_compute_subnetwork" "private_subnet" {
  name          = "private-subnet"
  network       = google_compute_network.vpc.id
  ip_cidr_range = "10.0.2.0/24"
}

# Create GKE cluster
resource "google_container_cluster" "kubeip_cluster" {
  name     = var.cluster_name
  location = var.region

  initial_node_count       = 1
  remove_default_node_pool = true

  # Enable VPC-native
  network    = google_compute_network.vpc.id
  subnetwork = google_compute_subnetwork.public_subnet.id
}

# Create node pools
resource "google_container_node_pool" "public_node_pool" {
  name       = "public-node-pool"
  location   = google_container_cluster.kubeip_cluster.location
  cluster    = google_container_cluster.kubeip_cluster.name
  node_count = 1
  autoscaling {
    min_node_count  = 1
    max_node_count  = 3
    location_policy = "ANY"
  }
  node_config {
    machine_type = "e2-medium"
    spot         = true
    labels       = {
      nodegroup = "public"
      kubeip    = "use"
    }
    resource_labels = {
      Environment = "demo"
      kubeip      = "use"
      public      = "true"
    }
  }
  node_locations = [google_compute_subnetwork.public_subnet.region]
}

resource "google_container_node_pool" "private_node_pool" {
  name       = "private-node-pool"
  location   = google_container_cluster.kubeip_cluster.location
  cluster    = google_container_cluster.kubeip_cluster.name
  node_count = 1
  autoscaling {
    min_node_count  = 1
    max_node_count  = 3
    location_policy = "ANY"
  }
  node_config {
    machine_type = "e2-medium"
    spot         = true
    labels       = {
      nodegroup = "private"
      kubeip    = "ignore"
    }
    resource_labels = {
      Environment = "demo"
      kubeip      = "ignore"
      public      = "false"
    }
  }
  network_config {
    enable_private_nodes = true
  }
  node_locations = [google_compute_subnetwork.private_subnet.region]
}

# Create static public IP addresses
resource "google_compute_address" "static_ip" {
  provider     = google-beta
  project      = var.project_id
  count        = 5
  name         = "static-ip-${count.index}"
  address_type = "EXTERNAL"
  region       = google_container_cluster.kubeip_cluster.location
  purpose      = "GCE_ENDPOINT"
  labels       = {
    kubeip = "reserved"
  }
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

# Configure Workload Identity for Kube IP service account and Kubernetes service account
module "kubeip-workload-identity" {
  source       = "terraform-google-modules/kubernetes-engine/google//modules/workload-identity"
  project_id   = var.project_id
  cluster_name = var.cluster_name
  location     = var.region
  name         = google_service_account.kubeip_service_account.name
  namespace    = "kube-system"
  depends_on   = [
    google_service_account.kubeip_service_account,
    google_service_account.kubeip_service_account,
    google_container_cluster.kubeip_cluster
  ]
}


# Deploy KubeIP DaemonSet
#resource "kubernetes_daemonset" "kubeip_daemonset" {
#  metadata {
#    name      = "kubeip-agent"
#    namespace = "kube-system"
#    labels    = {
#      app = "kubeip"
#    }
#  }
#  spec {
#    selector {
#      match_labels = {
#        app = "kubeip"
#      }
#    }
#    template {
#      metadata {
#        labels = {
#          app = "kubeip"
#        }
#      }
#      spec {
#        service_account_name = "kubeip-service-account"
#        container {
#          name  = "kubeip-agent"
#          image = "doitintl/kubeip-agent"
#        }
#        node_selector = {
#          nodegroup = "public"
#        }
#      }
#    }
#  }
#}
