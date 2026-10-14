# GCP lab validation

Target project: `thorson-lab1-sharedvpc`. Use a disposable GKE node pool and reserved public IPv4 addresses. Select the cluster, region, node pool, service account and reservation label filter before deploying. Read-only discovery on October 14, 2026 returned no GKE clusters in this project. A disposable cluster or another selected cluster project is required before live validation. No live lab resources have been changed by this preparation.

Use an image built from this candidate branch, not 2.3.0. Set `AWS_IPAM_POOL_ID=""` throughout GCP validation. The AWS IPAM runbook is separate.

## Baseline and assignment label (#126)

Record Node UID, provider ID, labels, taints, existing public address and reservation ownership. Set node-pool template labels so replacement Nodes inherit the DaemonSet selector. Enable `LABEL_ASSIGNED_ADDRESS=true` and Node patch permission. Assign a reserved address and verify the legacy `kubip_assigned` label has dashed IPv4, only after configured Node-address readiness checks. Test release, restart and a denied Node patch. Unrelated labels and annotations must remain unchanged. See [v1 migration](v1-labels.md) for #154.

## Metrics (#147)

Set `METRICS_ADDRESS=:8080` and scrape the pod directly or through a separately configured Service. The chart exposes the environment variable; it does not install Prometheus, a ServiceMonitor, or an unauthenticated public load balancer. Restrict access using the cluster's network controls.

The endpoint provides `/metrics`, `/healthz` (process liveness), and `/readyz` (configured address readiness). Readiness starts false and becomes true only after initial readiness, label and readiness-taint processing. Without drift repair, readiness reflects startup completion, not continuous cloud verification. An exhausted completion-mode node remains live but not ready. Use `/healthz` for liveness; using assignment readiness for liveness causes restart loops during exhaustion.

Counters describe assignment attempts and failures in the current process and reset on restart. Inventory refreshes every 30 seconds and reports matching existing reservations: available, assigned and total. Counts come from successive cloud queries and are approximate during concurrent changes. Scrapes do not make cloud API calls. Watch `kubeip_pool_inventory_up` and `kubeip_pool_inventory_timestamp_seconds`; a failed refresh retains the old snapshot and marks it unhealthy. Unsupported providers expose `kubeip_pool_inventory_supported=0`. Inventory supports GCP and AWS. AWS IPAM counts existing tagged EIPs, not unallocated CIDR capacity.

Each agent reports its view of the same configured pool. Deduplicate with `max` over targets sharing the same project, region and filter; do not sum replicas. Use scrape target labels to identify the Node and pool. Validate success, exhaustion, API failure, recovery, concurrent scrapes and process shutdown.

## Drift repair (#150)

Set `RECONCILE_INTERVAL=30s`, `RELEASE_ON_EXIT=false`, and Node patch permission. Keep `COMPLETION_TAINT_KEY=""`. Drift repair currently supports GCP. The first assigned reservation is persisted in `kubeip.com/assignment-state`; retries and restarts restore that address instead of selecting another one. A different instance or mode, invalid state or a reservation no longer matching the filter causes an error.

After initial assignment, remove the managed access configuration, then test replacing it with an ephemeral address. The next check must restore the original reservation. The assignment label is cleared before repair and restored after the configured Node-status check. Readiness becomes false during repair or observation failure. Move the desired reservation to another disposable instance and verify the agent refuses to take it; it must not remove the current address when the desired reservation is already unavailable. Inject a delete-operation wait error and verify no add follows an uncertain delete.

A cloud address change can interrupt running connections. The existing readiness taint is not re-added by the loop and a NoSchedule taint would not protect running pods. Use disposable workloads for these tests. The existing Kubernetes Lease serializes repairs with assignment, but cloud operations can still fail or be delayed, so exercise concurrent agents and failure recovery in the lab.

## Completion taint (#156)

Set a separate `COMPLETION_TAINT_KEY`, for example `kubeip.com/pending`, together with the existing readiness `TAINT_KEY`. Configure both taints on the node-pool template. Set `RELEASE_ON_EXIT=false` and leave drift repair and IPAM disabled. Node patch permission is required. Completion mode supports providers with read-only address observation (GCP and AWS).

For a mixed pool, ordinary workloads should tolerate the readiness taint but not the completion taint. Workloads requiring a reserved IP should tolerate neither. KubeIP itself must tolerate both.

A successful cloud assignment persists a terminal result before the completion taint is removed. The readiness taint is removed only after configured readiness checks and labeling succeed. If every attempt reports no available reservations, the agent persists exhaustion, removes only the completion taint, and stays running. It does not retry after a restart, even if an address later becomes available. Cloud permission errors, request timeouts and other uncertain failures keep the completion taint because they do not prove that all address changes have finished.

Test a pool with more disposable nodes than addresses. Successful nodes should lose both taints; exhausted nodes should lose only completion. Restart both kinds and verify no additional cloud assignment or release occurs. Change a completed node's address externally and verify restart refuses to modify it. Deny state persistence and confirm no completion taint is removed. Delete-operation and Kubernetes patch failures must preserve unrelated resources and taints.

Before changing modes, resetting terminal state or intentionally choosing a new address, drain the node, stop its agent, restore the appropriate template taints, and remove only `kubeip.com/assignment-state` after inspecting it. Restart and verify the result before uncordoning. Do not automate blind state resets.

## Acceptance and rollback

Keep all six issues open until their relevant lab scenarios pass. Record candidate image digest, configuration, Node UID, addresses, cloud operation IDs and observed results. Rollback means stopping the candidate, inspecting its persisted state and owned reservations, restoring a known configuration and verifying Node readiness before admitting workloads. Retained reservations are intentional in persistent modes and require explicit cleanup after disposable nodes are removed.
