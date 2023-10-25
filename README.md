![build](https://github.com/doitintl/kubeip/workflows/build/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/kubeip)](https://goreportcard.com/report/github.com/doitintl/kubeip) ![Docker Pulls](https://img.shields.io/docker/pulls/doitintl/kubeip-agent)

# KubeIP v2

Welcome to KubeIP v2, a complete overhaul of the popular [DoiT](https://www.doit.com/)
KubeIP [v1-main](https://github.com/doitintl/kubeip/tree/v1-main) open-source project, originally developed
by [Aviv Laufer](https://github.com/avivl).

KubeIP v2 expands its support beyond Google Cloud (as in v1) to include AWS, and it's designed to be extendable to other cloud providers
that allow assigning static public IP to VMs. We've also transitioned from a Kubernetes controller to a standard DaemonSet, enhancing
reliability and ease of use.

## What happens with KubeIP v1

KubeIP v1 is still available in the [v1-main](https://github.com/doitintl/kubeip/tree/v1-main) branch. No further development is planned. We
will fix critical bugs and security issues, but we will not add new features.

## What KubeIP v2 does?

Kubernetes' nodes don't necessarily need their own public IP addresses to communicate. However, there are certain situations where it's
beneficial for nodes in a node pool to have their own unique public IP addresses.

For instance, in gaming applications, a console might need to establish a direct connection with a cloud virtual machine to reduce the
number of hops.

Similarly, if you have multiple agents running on Kubernetes that need a direct server connection, and the server needs to whitelist all
agent IPs, having dedicated public IPs can be useful. These scenarios, among others, can be handled on a cloud-managed Kubernetes cluster
using Node Public IP.

KubeIP is a utility that assigns a static public IP to each node it manages. The IP is allocated to the node's primary network interface,
chosen from a pool of reserved static IPs using platform-supported filtering and ordering.

If there are no static public IPs left, KubeIP will hold on until one becomes available. When a node is removed, KubeIP releases the static
public IP back into the pool of reserved static IPs.

## How to use KubeIP?

Deploy KubeIP as a DaemonSet on your desired nodes using standard
Kubernetes [mechanism](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/). Once deployed, KubeIP will assign a static
public IP
to each node it operates on. If no static public IP is available, KubeIP will wait until one becomes available. When a node is deleted,
KubeIP will release the static public IP.

### IPv6 Support

KubeIP supports dual-stack IPv4/IPv6 GKE clusters and Google Cloud static public IPv6 addresses.
To enable IPv6 support, set the `ipv6` flag (or set `IPV6` environment variable) to `true` (default is `false`).

### Kubernetes Service Account

KubeIP requires a Kubernetes service account with the following permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubeip-service-account
  namespace: kube-system
---

piVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubeip-cluster-role
rules:
  - apiGroups: [ "" ]
    resources: [ "nodes" ]
    verbs: [ "get" ]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubeip-cluster-role-binding
subjects:
  - kind: ServiceAccount
    name: kubeip-service-account
    namespace: kube-system
roleRef:
  kind: ClusterRole
  name: kubeip-cluster-role
  apiGroup: rbac.authorization.k8s.io
```

### Kubernetes DaemonSet

Deploy KubeIP as a DaemonSet on your desired nodes using standard Kubernetes selectors. Once deployed, KubeIP will assign a static public IP
to the node's primary network interface, selected from a list of reserved static IPs using platform-supported filtering. If no static public
IP is available, KubeIP will wait until one becomes available. When a node is deleted, KubeIP will release the static public IP.

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kubeip
spec:
  selector:
    matchLabels:
      app: kubeip
  template:
    metadata:
      labels:
        app: kubeip
    spec:
      serviceAccountName: kubeip-service-account
      terminationGracePeriodSeconds: 30
      priorityClassName: system-node-critical
      nodeSelector:
        kubeip.com/public: "true"
      containers:
        - name: kubeip
          image: doitintl/kubeip-agent
          resources:
            requests:
              cpu: 100m
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: FILTER
              value: PUT_PLATFORM_SPECIFIC_FILTER_HERE
            - name: LOG_LEVEL
              value: debug
            - name: LOG_JSON
              value: "true"
```

### AWS

Make sure that KubeIP DaemonSet is deployed on nodes that have a public IP (node running in public subnet) and uses a Kubernetes service
account [bound](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) to the IAM role with the following
permissions:

```yaml
Version: '2012-10-17'
Statement:
  - Effect: Allow
    Action:
      - ec2:AssociateAddress
      - ec2:DisassociateAddress
      - ec2:DescribeInstances
      - ec2:DescribeAddresses
    Resource: '*'
```

KubeIP supports filtering of reserved Elastic IPs using tags and Elastic IP properties. To use this feature, add the `filter` flag (or
set `FILTER` environment variable) to the KubeIP DaemonSet:

```yaml
- name: FILTER
  value: "Name=tag:env,Values=dev;Name=tag:app,Values=streamer"
```

KubeIP AWS filter supports the same filter syntax as the AWS `describe-addresses` command. For more information,
see [describe-addresses](https://docs.aws.amazon.com/cli/latest/reference/ec2/describe-addresses.html#options). If you specify multiple
filters, they are joined with an `AND`, and the request returns only results that match all the specified filters. Multiple filters must be
separated by semicolons (`;`).

### Google Cloud

Ensure that the KubeIP DaemonSet is deployed on nodes with a public IP (nodes in a public subnet) and uses a Kubernetes service
account [bound](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity) to an IAM role with the following permissions:

```yaml
title: "KubeIP Role"
description: "KubeIP required permissions"
stage: "GA"
includedPermissions:
  - compute.instances.addAccessConfig
  - compute.instances.deleteAccessConfig
  - compute.instances.get
  - compute.addresses.get
  - compute.addresses.list
  - compute.addresses.use
  - compute.zoneOperations.get
  - compute.subnetworks.useExternalIp
  - compute.projects.get
```

KubeIP Google Cloud filter supports the same filter syntax as the Google Cloud `gcloud compute addresses list` command. For more
information, see [gcloud topic filter](https://cloud.google.com/sdk/gcloud/reference/topic/filters). If you specify multiple filters, they
are joined with an `AND`, and the request returns only results that match all the specified filters. Multiple filters must be separated by
semicolons (`;`).

To use this feature, add the `filter` flag (or set `FILTER` environment variable) to the KubeIP DaemonSet:

```yaml
- name: FILTER
  value: "labels.env=dev;labels.app=streamer"
```

## How to contribute to KubeIP?

KubeIP is an open-source project, and we welcome your contributions!

## How to build KubeIP?

KubeIP is written in Go and can be built using standard Go tools. To build KubeIP, run the following command:

```shell
make build
```

## How to run KubeIP?

KubeIP is a standard command-line application. To explore the available options, run the following command:

```shell
kubeip-agent run --help
```

```text
NAME:
   kubeip-agent run - run agent

USAGE:
   kubeip-agent run [command options] [arguments...]

OPTIONS:
   Configuration

   --filter value [ --filter value ]  filter for the IP addresses [$FILTER]
   --ipv6                             enable IPv6 support (default: false) [$IPV6]
   --kubeconfig value                 path to Kubernetes configuration file (not needed if running in node) [$KUBECONFIG]
   --node-name value                  Kubernetes node name (not needed if running in node) [$NODE_NAME]
   --order-by value                   order by for the IP addresses [$ORDER_BY]
   --project value                    name of the GCP project or the AWS account ID (not needed if running in node) [$PROJECT]
   --region value                     name of the GCP region or the AWS region (not needed if running in node) [$REGION]
   --release-on-exit                  release the static public IP address on exit (default: true) [$RELEASE_ON_EXIT]
   --retry-attempts value             number of attempts to assign the static public IP address (default: 10) [$RETRY_ATTEMPTS]
   --retry-interval value             when the agent fails to assign the static public IP address, it will retry after this interval (default: 5m0s) [$RETRY_INTERVAL]

   Development

   --develop-mode  enable develop mode (default: false) [$DEV_MODE]

   Logging

   --json             produce log in JSON format: Logstash and Splunk friendly (default: false) [$LOG_JSON]
   --log-level value  set log level (debug, info(*), warning, error, fatal, panic) (default: "info") [$LOG_LEVEL]
```

## How to test KubeIP?

To test KubeIP, create a pool of reserved static public IPs, ensuring that the pool has enough IPs to assign to all nodes that KubeIP will
operate on. Use labels to filter the pool of reserved static public IPs.

Next, create a Kubernetes cluster and deploy KubeIP as a DaemonSet on your desired nodes. Ensure that the nodes have a public IP (nodes in a
public subnet). Configure KubeIP to use the pool of reserved static public IPs, using filters and order by.

Finally, scale the number of nodes in the cluster and verify that KubeIP assigns a static public IP to each node. Scale down the number of
nodes in the cluster and verify that KubeIP releases the static public IP addresses.

#### AWS EKS Example

The [examples/aws](examples/aws) folder contains a Terraform configuration that creates an EKS cluster and deploys KubeIP as a DaemonSet on
the cluster nodes in a public subnet. The Terraform configuration also creates a pool of reserved Elastic IPs and configures KubeIP to use
the pool of reserved static public IPs.

To run the example, follow these steps:

```shell
cd examples/aws
terraform init
terraform apply
```

#### Google Cloud GKE Example

The [examples/gcp](examples/gcp) folder contains a Terraform configuration that creates a GKE cluster and deploys KubeIP as a DaemonSet on
the cluster nodes in a public subnet. The Terraform configuration also creates a pool of reserved static public IPs and configures KubeIP to
use the pool of reserved static public IPs.

To run the example, follow these steps:

```shell
cd examples/gcp
terraform init
terraform apply -var="project_id=<your-project-id>"
```

To run the example with GKE dual-stack IPv4/IPv6 cluster, follow these steps:

```shell
cd examples/gcp
terraform init
terraform apply -var="project_id=<your-project-id>" -var="ipv6_support=true"
```