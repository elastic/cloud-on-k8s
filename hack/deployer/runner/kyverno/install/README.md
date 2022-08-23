# Enforcing Security Policies with Kyverno

This directory holds the files used to install and setup [Kyverno](https://kyverno.io/). Kyverno is used to enforce basic security policies when running the E2E test pipelines.

## How to update Kyverno?

The deployer uses the [`kyverno.yaml`](kyverno.yaml) manifest located in this directory to deploy Kyverno as soon as the `psp` property is set to `true`, for example:

```yaml
id: kind-dev
overrides:
  clusterName: my-dev-cluster
  psp: true
```


Kyverno can be updated from the Kyverno project using the following command:

```
wget -O kyverno.yaml https://raw.githubusercontent.com/kyverno/kyverno/release-1.7/config/release/install.yaml`
```

(replace `1.7` by the expected version)

## Security Policies

The [`policies`](policies.yaml) manifest contains basic security policies. You can find more examples [here](https://kyverno.io/policies/?policytypes=Pod%2520Security%2520Standards%2520%28Restricted%29%2Bvalidate). Agent and Beat Pods are excluded as they generally require advanced privileges to function properly.