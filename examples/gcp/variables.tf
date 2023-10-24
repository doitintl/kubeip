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

variable "subnet_range" {
  type    = string
  default = "10.128.0.0/20"
}

variable "pods_range" {
  type    = string
  default = "10.128.64.0/18"
}

variable "pods_range_name" {
  type    = string
  default = "pods-range"
}

variable "services_range_name" {
  type    = string
  default = "services-range"
}

variable "services_range" {
  type    = string
  default = "10.128.32.0/20"
}

variable "machine_type" {
  type    = string
  default = "e2-medium"
}

variable "ipv6_support" {
  type    = bool
  default = false
}