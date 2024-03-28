# ECK Operator, and ECK Resources Helm Charts

[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/elastic)](https://artifacthub.io/packages/search?repo=elastic)

This directory contains the Helm chart for deploying the ECK operator, and charts for deploying any resource in the Elastic Stack individually, or as a group.

The instructions below are intended to deploy the Helm charts from a local copy of this repository. Refer to https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-install-helm.html to install the charts from the official repository.

## ECK Operator Helm Chart Usage

View the available settings for customizing the installation.

```sh
helm show values ./eck-operator
```

Install the CRDs and deploy the operator with cluster-wide permissions to manage all namespaces.

```sh
helm install elastic-operator ./eck-operator -n elastic-system --create-namespace
```

Install the operator restricted to a single namespace.

```sh
# This step must be done by a cluster administrator to install the CRDs -- which are global resources.
helm install elastic-operator-crds ./eck-operator/charts/eck-operator-crds

# This step can be done by any user with full access to the my-namespace namespace.
helm install elastic-operator ./eck-operator -n my-namespace --create-namespace \
  --set=installCRDs=false \
  --set=managedNamespaces='{my-namespace}' \
  --set=createClusterScopedResources=false \
  --set=webhook.enabled=false
```

## ECK Stack Helm Chart Usage

Install a quickstart Elasticsearch and Kibana resource in a cluster controlled by the ECK Operator.

```sh
helm install es-kb-quickstart ./eck-stack -n elastic-stack --create-namespace
```

To see all resources installed by the helm chart:

```sh
kubectl get elastic -l "app.kubernetes.io/instance"=es-kb-quickstart -n elastic-stack
```

## ECK Helm Chart Development

### ECK Helm Chart test suite

[Helm UnitTest Plugin](https://github.com/quintush/helm-unittest) is used to ensure Helm Charts render properly.

#### Installation

```
helm plugin install https://github.com/quintush/helm-unittest --version 0.2.8
```

#### Running Test Suite

The test suite can be run from the Makefile in the root of the project with the following command:

```
make helm-test
```

*Note* that the Makefile target runs the script in `{root}/hack/helm/test.sh`

#### Manually invoking the Helm Unit Tests for a particular Chart

The Helm unit tests can be manually invoked for any of the charts with the following command:

```
cd deploy/eck-stack
helm unittest -3 -f 'templates/tests/*.yaml' --with-subchart=false .
```

## Licensing

The ECK Helm Charts are licensed under the [Elastic License 2.0](https://www.elastic.co/licensing/elastic-license) like the operator. They can be used with a Basic license for free.
