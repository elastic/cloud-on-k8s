# ECK-Stack

## TODO

- [ ] Write documentation in the official ECK /docs directory.
- [ ] Document using examples values.

ECK Stack is a Helm chart to assist in the deployment of Elastic Stack components, which are
managed by the [ECK Operator](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html)

## Supported Elastic Stack Resources

The following Elastic Stack resources are currently supported. 

- Elasticsearch
- Kibana

Additional resources will be supported in future releases of this Helm Chart.

## Prerequisites

- Kubernetes 1.20+
- Elastic ECK Operator

## Installing the Chart

### Installing the ECK Operator

Before using this chart, the Elastic ECK Operator is required to be installed within the Kubernetes cluster.
Full installation instructions can be found within the [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-installing-eck.html)

To install the ECK Operator using Helm.

```sh
# Add the Elastic Helm Repository
helm repo add elastic https://helm.elastic.co && helm repo update
# Install the ECK Operator cluster-wide
helm install elastic-operator elastic/eck-operator -n elastic-system --create-namespace
```

Additional ECK Operator Helm installation options can be found within the [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-install-helm.html)

### Installing the ECK Stack Chart

The following will install the ECK-Stack chart using the default values, which will deploy an Elasticsearch [Quickstart Cluster](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-elasticsearch.html)

```sh
# Add the Elastic Helm Repository
helm repo add elastic https://helm.elastic.co && helm repo update

# Install the ECK-Stack helm chart
# This will setup a 'quickstart' Elasticsearch cluster
$ helm install my-release --namespace my-namespace elastic/eck-stack
```

More information on the different ways to use the ECK Stack chart to deploy Elastic Stack resources
can be found in [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html).

> **Tip**: List all releases in all Kubernetes namespaces using `helm list -A`

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment:

```console
$ helm delete my-release
```

The command removes all the Elastic Stack resources associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the cert-manager chart and their default values.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `eck-elasticsearch.enabled` | If `true`, create an Elasticsearch cluster (using the eck-elasticsearch Chart) | `true` |
| `eck-kibana.enabled` | If `true`, create a Kibana instance (using the eck-kibana Chart) | `false` |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
$ helm install my-release -f values.yaml .
```

## Contributing

This chart is maintained at [github.com/elastic/cloud-on-k8s](https://github.com/elastic/cloud-on-k8s/tree/main/deploy/eck-stack).