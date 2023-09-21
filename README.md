![ci](https://github.com/doitintl/kubeip/workflows/ci/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/kubeip)](https://goreportcard.com/report/github.com/doitintl/kubeip) ![Docker Pulls](https://img.shields.io/docker/pulls/doitintl/kubeip)

# What is kubeIP?

Many applications need to be whitelisted by users based on a Source IP Address. As of today, cloud-manages Kubernetes Engines do not support
assigning a static pool of IP addresses to the Kubernetes cluster. Using kubeIP, this problem is solved by assigning Kubernetes nodes
external IP addresses from a predefined list.

