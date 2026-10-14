# AWS VPC IPAM lab candidate

Issue #166 uses AWS VPC IPAM public IPv4 pools. It is excluded from the first GCP pass in `thorson-lab1-sharedvpc`. This feature is not in agent 2.3.0. Use an image built from the candidate branch.

Set `AWS_IPAM_POOL_ID=ipam-pool-...` to allocate an Elastic IP from an existing public IPv4 IPAM pool. Leave it empty to retain the existing preallocated-address behavior. IPAM mode requires AWS, IPv4, empty `FILTER` and `ORDER_BY`, and Kubernetes Node `get` and `patch` permission. The target must have a primary network interface. IPAM recovery also supports an interface without a public IPv4 address, for example after release on an EKS node with multiple interfaces. The pod needs an independent path to EC2 and Kubernetes APIs during release and recovery, such as an EC2 interface VPC endpoint with private DNS and the private EKS endpoint. Image pulls and credential retrieval also need connectivity if the node has no public address. With EKS Pod Identity, provide the `com.amazonaws.<region>.eks-auth` interface endpoint as well as the EC2 endpoint. Cache or privately host the image, or provide independent outbound access. KubeIP does not provision the IPAM pool.

The pool must have a public IPv4 CIDR provisioned, support EC2 allocations, and match the target region/network border group. AWS describes these requirements in [Allocate sequential Elastic IP addresses from an IPAM pool](https://docs.aws.amazon.com/vpc/latest/ipam/tutorials-eip-pool.html). The existing AWS identity also needs `ec2:AllocateAddress`, `ec2:AllocateIpamPoolCidr` on the pool, `ec2:CreateTags` during allocation, and `ec2:ReleaseAddress`, in addition to the existing describe, associate and disassociate permissions. Restrict allocation to the lab pool and release/association to resources carrying the ownership tags. Use the pool ARN returned by AWS; IPAM pool ARNs omit the region (`arn:aws:ec2::<account>:ipam-pool/<pool-id>`). Verify the complete policy with the lab account before deployment.

## Ownership and recovery

Before calling `AllocateAddress`, KubeIP writes `kubeip.com/ipam-allocation` on its Node with the pool and instance ID. The write uses the Node UID and resource version to prevent concurrent allocation claims. The EIP is created with `kubeip.com/instance` and `kubeip.com/ipam-pool` tags. These tags are reserved for KubeIP ownership; do not add them to unrelated EIPs or change them while the agent runs.

[AllocateAddress](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_AllocateAddress.html) has no idempotency token. KubeIP disables SDK retries for this call. If its response is lost, later attempts discover the allocation by its tags. If no allocation is visible but intent exists, KubeIP stops allocating. This also applies to a definite allocation rejection: resolve the cause and inspect AWS before manually resetting intent. A crash after saving intent but before making the request requires the same recovery procedure.

Failed association retains the reservation for retry. It does not release an address after an ambiguous association response. A restart reuses the allocation, including an already associated primary address. Multiple matching allocations, a changed pool or instance, an unrelated existing primary EIP, or an owned address associated elsewhere cause an error without stealing or replacing that allocation.

`RELEASE_ON_EXIT=false` retains the reservation and intent for restart. With the default `true`, a normal shutdown disassociates and releases only the tagged allocation, then clears intent. A failed release keeps intent. A lost release response or failed intent cleanup may require manual reconciliation. Missing Node state, abrupt instance deletion and exhausted startup retries do not trigger account-wide cleanup. Monitor retained allocations for charges and clean them up explicitly after checking ownership.

To recover unresolved intent, stop the agent on the affected node, check EC2/CloudTrail for the allocation request, and list EIPs using both ownership tags. Allow for eventual consistency and confirm there is no in-flight request. Reuse a discovered allocation. Only if allocation definitively did not occur, or the owned allocation has been released, remove that Node's `kubeip.com/ipam-allocation` annotation before restarting. Do not automate blind annotation resets.

## AWS validation matrix

The selected AWS account is `476383094422`, using local profile `thorson-sandbox1-admin`. The October 14 run used disposable resources in `us-west-2`; see [live results and deployment requirements](aws-2026-10-14.md). The matrix below remains the acceptance checklist for future runs.

1. Confirm candidate image, role permissions, empty filters, Node patch permission, and unused pool capacity. Record existing unrelated EIPs.
2. Start one disposable node. Verify one allocation from the selected pool, both ownership tags, the Node intent, and primary-interface association.
3. With release disabled, restart the pod repeatedly. Allocation ID and public address must remain unchanged.
4. Deny association after allocation. Retry and restart. Exactly one reserved allocation must remain, and retries must use it. Restore permission and confirm assignment.
5. Simulate a lost allocation response. Confirm retries recover by tags. While visibility is delayed, no second allocation may occur.
6. Deny allocation or exhaust pool capacity. Confirm no workload-ready marker is published. Inspect and reset intent manually after correcting the problem.
7. Attach an unrelated EIP to the primary interface, or move the owned address elsewhere. Confirm the agent refuses reassociation and release of unrelated addresses.
8. Enable release and stop the pod normally. Confirm disassociation, release to the pool, and intent removal. Repeat with denied release and confirm intent persists until recovery.
9. Verify unrelated EIPs, node labels and annotations remain unchanged. Remove only the lab's owned resources after verifying IDs and ownership.

Unit tests cover allocation request tags, disabled SDK retries, restart reuse, delayed discovery after a lost response, retained allocation after association failure, ownership refusal, state/configuration errors, release failure, and Node patch preconditions. Live results and limitations are recorded in the [AWS lab report](aws-2026-10-14.md).
