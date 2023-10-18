variable "region" {
  type    = string
  default = "us-west-2"
}

variable "availability_zones" {
  type    = list(string)
  default = ["us-west-2a", "us-west-2b", "us-west-2c"]
}

variable "cluster_name" {
  type    = string
  default = "kubeip-demo"
}

variable "vpc_name" {
  type    = string
  default = "kubeip-demo"
}

variable "vpc_cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "private_cidr_ranges" {
  type    = list(string)
  default = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
}

variable "public_cidr_ranges" {
  type    = list(string)
  default = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]
}

variable "kubernetes_version" {
  type    = string
  default = "1.28"
}