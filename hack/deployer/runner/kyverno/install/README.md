# Enforcing Security Policies with Kyverno

This directory holds the files used to install and setup [Kyverno](https://kyverno.io/). Kyverno is used to enforce basic security policies when running the E2E test pipelines.

## How to update Kyverno?

The deployer uses the [`kyverno.yaml`](kyverno.yaml) manifest located in this directory to deploy Kyverno as soon as the `enforceSecurityPolicies` property is **not** set to `false`, for example:

```yaml
id: kind-dev
overrides:
  clusterName: my-dev-cluster
  enforceSecurityPolicies: true
```


Kyverno can be updated from the Kyverno project using the following command:

```shell
wget -O kyverno.yaml https://github.com/kyverno/kyverno/releases/download/v1.17.1/install.yaml
```

(replace `1.17.1` by the expected version)

## Security Policies

The [`policies`](policies.yaml) manifest contains basic security policies. You can find more examples [here](https://kyverno.io/policies/?policytypes=Pod%2520Security%2520Standards%2520%28Restricted%29%2Bvalidate). Agent and Beat Pods are excluded as they generally require advanced privileges to function properly.