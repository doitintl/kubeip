# Remaining issue preparation

Target for the first lab pass: GCP project `thorson-lab1-sharedvpc`. No deployment is authorized by this preparation step. Cluster, node pool, reserved address pool, and disposable workload must be selected before running destructive scenarios.

| Issue | Candidate behavior | Lab acceptance |
| --- | --- | --- |
| #126 | Opt-in legacy assignment label, with patches limited to its own key. | IPv4 label, restart restoration, patch failure, release, unrelated labels preserved. |
| #147 | Opt-in Prometheus endpoint with assignment state, attempt/error counters, pool inventory, and health/readiness endpoints. | Scrape during success, exhaustion, cloud API failure and shutdown; do not sum replicated pool counts. |
| #150 | Opt-in GCP reconciliation of the persisted assigned address under the existing lease. | Remove or replace the managed address; restore the original reservation; refuse to steal an address attached elsewhere. |
| #154 | V1 cleanup diagnosis and migration guidance; preserve custom labels in v2 tests. | Compare Node UID across replacement; use node-pool template labels; verify custom labels survive assignment and release. |
| #156 | Separate completion taint plus durable terminal state on the Node. Require RELEASE_ON_EXIT=false and disable drift repair in this mode. | Exhaust the pool, remove completion taint but retain readiness taint; restart agent without more cloud mutations; drain before resetting terminal state. |
| #166 | Opt-in AWS public IPv4 IPAM allocation, durable allocation intent, recovery by ownership tags, and release of owned allocations. | Excluded from GCP tests. Validate separately in the AWS lab account: allocation, restart, lost responses, association failure, pool exhaustion and release. |

All behavior-changing options default off. New features are lab candidates, not part of agent 2.3.0. Do not publish or close these issues before lab validation.

AWS IPAM implementation and validation details: [AWS IPAM lab](aws-ipam.md).
