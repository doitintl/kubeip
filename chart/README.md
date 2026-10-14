# KubeIP Helm chart

Install from a local checkout:

```sh
helm upgrade --install kubeip ./chart --namespace kube-system --values my-values.yaml
```

Set `image.repository` to the image name without a tag and `image.tag` to the desired
agent version. Configure `daemonSet.nodeSelector` and the provider credentials for
your cluster before installing.

When using taints, set `daemonSet.env.TAINT_KEY` and
`rbac.allowNodesPatchPermission=true`. `daemonSet.env.WAIT_FOR_NODE_ADDRESS` defaults
to true. Set it to false only when workloads can start after cloud assignment,
without waiting for Node status to advertise the IP. This option requires an agent
release containing the `wait-for-node-address` flag.

## Publishing

The chart workflow validates and packages chart changes. To publish a release:

1. Update `version` in `Chart.yaml` and merge the validated change.
2. Push a tag named `chart-v<VERSION>` on that commit, for example `chart-v0.2.0`.
3. The workflow checks that the tag matches the chart version, then publishes the
   package to `ghcr.io/doitintl/kubeip/charts/kubeip` using the workflow token.
4. For the first release, make the GHCR package public and verify an anonymous pull.
   Organization policies must permit package creation by this repository.
5. Register `oci://ghcr.io/doitintl/kubeip/charts/kubeip` as a Helm repository in
   Artifact Hub. Artifact Hub indexes the registry; it does not host the package.

After a version is published, users can install it without cloning the repository:

```sh
helm upgrade --install kubeip oci://ghcr.io/doitintl/kubeip/charts/kubeip \
  --version 0.2.0 --namespace kube-system --values my-values.yaml
```

The release workflow and registry setup must complete before that command works.
See the [Artifact Hub instructions](https://artifacthub.io/docs/topics/repositories/helm-charts/)
for registration and optional publisher verification.
