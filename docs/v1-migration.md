# Node selector labels and v1 migration (#154)

The preserved v1 branch (`6106519ec0625191f917bbff810342d2f07f0e3a`) defaults `CopyLabels` and `ClearLabels` to true. Its `clearLabels` implementation removes labels outside a small allowlist. A custom selector such as `kubeip.com/public=true` is not preserved. The issue comments describe this v1 behavior and suggest disabling both options.

V2 does not run that broad cleanup. The optional legacy assignment-label feature patches only `kubip_assigned`; it does not copy labels from cloud addresses or remove arbitrary node labels. The IPAM and persistent-assignment state patches also modify only their own annotation keys. Regression tests preserve custom labels and unrelated annotations through assignment and cleanup.

To diagnose label loss on GCP:

1. Record the Node UID, creation timestamp, selector labels and running KubeIP image before and after the event.
2. If the UID changed, the node was replaced. Put required selector labels on the GKE node-pool template so a replacement receives them. A manual label on an old Node does not configure its replacement.
3. If the UID stayed the same, inspect Kubernetes audit logs for the actor changing metadata. Check for a remaining v1 deployment or another label controller.
4. Remove the v1 controller before enabling v2 on the test pool. Do not run both controllers against the same nodes and address pool.
5. Confirm custom labels survive candidate assignment, restart, release, drift repair and completion-state updates.

This identifies a v1 cause, not a reproduced v2 label-deletion defect. Confirm the deployed version, Node identity and metadata actor before attributing label loss to v2.
