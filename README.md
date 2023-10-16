![build](https://github.com/doitintl/kubeip/workflows/build/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/kubeip)](https://goreportcard.com/report/github.com/doitintl/kubeip) ![Docker Pulls](https://img.shields.io/docker/pulls/doitintl/kubeip-agent)

# KubeIP v2

Welcome to KubeIP v2, a complete overhaul of the popular [DoiT](https://www.doit.com/)
KubeIP [v1](https://github.com/doitintl/kubeip/tree/v1-main) open-source project, originally developed
by [Aviv Laufer](https://github.com/avivl).

KubeIP v2 expands its support beyond Google Cloud (as in v1) to include AWS, and it's designed to be extendable to other cloud providers
that allow assigning static public IP to VMs. We've also transitioned from a Kubernetes controller to a standard DaemonSet, enhancing
reliability and ease of use.

## What KubeIP does?

KubeIP is a tool that assigns a static public IP to any node it operates on. The IP is assigned to the node's primary network interface,
selected from a list of reserved static IPs using platform-supported filtering.

## How to use KubeIP?

Deploy KubeIP as a DaemonSet on your desired nodes using standard Kubernetes selectors. Once deployed, KubeIP will assign a static public IP
to each node it operates on. If no static public IP is available, KubeIP will wait until one becomes available. When a node is deleted,
KubeIP will release the static public IP.

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

### AWS

Make sure that KubeIP DaemonSet is deployed on nodes that have a public IP (node in public subnet) and uses Kubernetes service
account [bound](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
to IAM role with the following permissions:

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

KubeIP supports filtering of reserved Elastic IPs using tags. To use this feature, add the `filter` flag (or set `FILTER` environment
variable) to the KubeIP DaemonSet:

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
      serviceAccountName: kubeip-sa
      nodeSelector:
        kubeip.com/public: "true"
      containers:
        - name: kubeip
          image: doitintl/kubeip-agent
          env:
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
  - compute.addresses.list
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
      serviceAccountName: kubeip-sa
      nodeSelector:
        kubeip.com/public: "true"
      containers:
        - name: kubeip
          image: doitintl/kubeip-agent
          env:
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
