# ECK-Stack

ECK Stack is a Helm chart to assist in the deployment of Elastic Stack components, which are
managed by the [ECK Operator](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html)

## Supported Elastic Stack Resources

The following Elastic Stack resources are currently supported. 

- Elasticsearch
- Kibana
- Elastic Agent
- Fleet Server
- Beats
- Logstash
- APM Server

Additional resources will be supported in future releases of this Helm Chart.

## Prerequisites

- Kubernetes 1.21+
- Elastic ECK Operator

## Installing the Chart

### Installing the ECK Operator

Before using this chart, the Elastic ECK Operator is required to be installed within the Kubernetes cluster.
Full installation instructions can be found within [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-installing-eck.html)

To install the ECK Operator using Helm.

```sh
# Add the Elastic Helm Repository
helm repo add elastic https://helm.elastic.co && helm repo update

# Install the ECK Operator cluster-wide
helm install elastic-operator elastic/eck-operator -n elastic-system --create-namespace
```

Additional ECK Operator Helm installation options can be found within [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-install-helm.html)

### Installing the ECK Stack Chart

The following will install the ECK-Stack chart using the default values, which will deploy an Elasticsearch [Quickstart Cluster](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-elasticsearch.html), and a Kibana [Quickstart Instance](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-deploy-kibana.html)

```sh
# Add the Elastic Helm Repository
helm repo add elastic https://helm.elastic.co && helm repo update

# Install the ECK-Stack helm chart
# This will setup a 'quickstart' Elasticsearch and Kibana resource in the 'elastic-stack' namespace
helm install my-release elastic/eck-stack -n elastic-stack --create-namespace
```

More information on the different ways to use the ECK Stack chart to deploy Elastic Stack resources
can be found in [our documentation](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html).

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment from the 'elastic-stack' namespace:

```console
helm delete my-release -n elastic-stack
```

The command removes all the Elastic Stack resources associated with the chart and deletes the release.

## Configuration

The following table lists the configurable parameters of the eck-stack chart and their default values.

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `eck-elasticsearch.enabled` | If `true`, create an Elasticsearch resource (using the eck-elasticsearch Chart) | `true` |
| `eck-kibana.enabled` | If `true`, create a Kibana resource (using the eck-kibana Chart) | `true` |
| `eck-agent.enabled` | If `true`, create an Elastic Agent resource (using the eck-agent Chart) | `false` |
| `eck-fleet-server.enabled` | If `true`, create a Fleet Server resource (using the eck-fleet-server Chart) | `false` |
| `eck-logstash.enabled` | If `true`, create a Logstash resource (using the eck-logstash Chart) | `false` |
| `eck-apm-server.enabled` | If `true`, create a standalone Elastic APM Server resource (using the eck-apm-server Chart) | `false` |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`.

Alternatively, a YAML file that specifies the values for the above parameters can be provided while installing the chart. For example,

```console
helm install my-release -f values.yaml .
```

## Contributing

This chart is maintained at [github.com/elastic/cloud-on-k8s](https://github.com/elastic/cloud-on-k8s/tree/main/deploy/eck-stack).
