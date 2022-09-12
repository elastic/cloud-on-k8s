# ECK Operator, and ECK Resources Helm Charts

[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/elastic)](https://artifacthub.io/packages/search?repo=elastic)

This directory contains the Helm chart for deploying the ECK operator, and charts for deploying any resource in the Elastic Stack individually, or as a group.

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

## ECK Stack Helm Chart Usage

Install a quickstart Elasticsearch and Kibana resource in a cluster controlled by the ECK Operator.

```sh
helm install es-kb-quickstart elastic/eck-stack -n elastic-stack --create-namespace
```

To see all resources installed by the helm chart

```sh
kubectl get elastic -l "app.kubernetes.io/instance"=es-kb-quickstart -n elastic-stack
```

## Licensing

### ECK Operator Helm Chart

The ECK Operator and its Helm Chart are licensed under the [Elastic License 2.0](https://www.elastic.co/licensing/elastic-license), and are free to use under the limitations defined within the License.

### ECK Stack/Resources Helm Charts

The ECK Stack and Resources (eck-stack, eck-elasticsearch, eck-kibana, etc) Helm Charts are also licensed under the [Elastic License 2.0](https://www.elastic.co/licensing/elastic-license), but require an [Elastic Enterprise](https://www.elastic.co/subscriptions) license for use.