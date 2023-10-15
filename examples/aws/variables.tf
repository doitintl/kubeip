variable "region" {
  type    = string
  default = "us-west-2"
}

variable "cluster_name" {
  type    = string
  default = "kubeip-demo"
}

variable "vpc_name" {
  type    = string
  default = "kubeip-demo"
}

variable "kubernetes_version" {
  type    = string
  default = "1.28"
}