# ECK Operator, and ECK Resources Helm Charts

[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/elastic)](https://artifacthub.io/packages/search?repo=elastic)

This directory contains the Helm charts for deploying the ECK operator, and for the resources managed by the ECK Operator.

## ECK Operator Helm Chart Usage

Install the CRDs and deploy the operator with cluster-wide permissions to manage all namespaces.

```sh
helm install elastic-operator eck-operator -n elastic-system --create-namespace 
```

Install the operator restricted to a single namespace. 

```sh
# This step must be done by a cluster administrator to install the CRDs -- which are global resources.
helm install elastic-operator-crds ./eck/charts/eck-operator-crds 

# This step can be done by any user with full access to the my-namespace namespace.
helm install elastic-operator eck-operator -n my-namespace --create-namespace \
  --set=installCRDs=false \
  --set=managedNamespaces='{my-namespace}' \
  --set=createClusterScopedResources=false \
  --set=webhook.enabled=false
```

View the available settings for customizing the installation.

```sh
helm show values eck-operator
```

## ECK Resources Helm Chart Usage

Install a basic Elasticsearch and Kibana instance in a cluster controlled by the ECK Operator.

```sh
helm install resources eck-resources -n default
```

Install a quickstart Elasticsearch and Kibana instance in a cluster controlled by the ECK Operator using specific values files.

```sh
helm install resources eck-resources -n default --values ./examples/elasticsearch/quickstart.yaml --values ./examples/kibana/quickstart.yaml
```
