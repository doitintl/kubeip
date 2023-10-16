variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "us-central1"
}

variable "cluster_name" {
  type    = string
  default = "kubeip-demo"
}

variable "vpc_name" {
  type    = string
  default = "kubeip-demo"
}

variable "machine_type" {
  type    = string
  default = "e2-medium"
}