# ECK Operator Helm Chart

[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/elastic)](https://artifacthub.io/packages/search?repo=elastic)

This directory contains the Helm chart for deploying the ECK operator.

See also the [ECK helm install guide](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-install-helm.html).

## Usage

Install the CRDs and deploy the operator with cluster-wide permissions to manage all namespaces.

```sh
helm install elastic-operator elastic/eck-operator -n elastic-system --create-namespace 
```

Install the operator restricted to a single namespace. 

```sh
# This step must be done by a cluster administrator to install the CRDs -- which are global resources.
helm install elastic-operator-crds ./eck/charts/eck-operator-crds 

# This step can be done by any user with full access to the my-namespace namespace.
helm install elastic-operator elastic/eck-operator -n my-namespace --create-namespace \
  --set=installCRDs=false \
  --set=managedNamespaces='{my-namespace}' \
  --set=createClusterScopedResources=false \
  --set=webhook.enabled=false
```

View the available settings for customizing the installation.

```sh
helm show values elastic/eck-operator
```


