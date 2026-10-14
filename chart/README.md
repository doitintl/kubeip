# KubeIP Helm chart

The maintained chart source and publishing workflow now live in
[doitintl/kubeip-helm-chart](https://github.com/doitintl/kubeip-helm-chart), following
the DoiT EKS Lens pattern of GitHub Releases and GitHub Pages.

## Install the published chart

```sh
helm repo add kubeip https://doitintl.github.io/kubeip-helm-chart/
helm repo update
helm upgrade --install kubeip kubeip/kubeip \
  --version 0.2.0 --namespace kube-system --values my-values.yaml
```

Chart 0.2.0 defaults to agent 2.3.0. Configure the cloud provider, credentials,
node selector, and optional taint settings in `my-values.yaml` for your cluster.
See the [maintained chart documentation](https://github.com/doitintl/kubeip-helm-chart#readme)
for configuration and publishing instructions.

This directory remains available for existing local-checkout users. Future chart
changes and versioned releases belong in the dedicated chart repository. This
repository no longer publishes charts to GHCR or on `chart-v*` tags.
