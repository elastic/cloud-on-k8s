# ECK-Integrations

ECK Integrations is a Helm chart to assist in the deployment of Elastic integrations, which are
managed by the [ECK Operator](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html)

## Supported Elastic Integrations

The following Elastic integrations are currently supported. 

- Kubernetes

Additional integrations will be supported in future releases of this Helm Chart.

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

### Installing Kubernetes ECK Integration

// TODO(panosk)

## Uninstalling the Chart

To uninstall/delete the `my-release` deployment from the 'elastic-stack' namespace:

```console
helm delete my-release -n elastic-stack
```

The command removes all the Elastic Stack resources associated with the chart and deletes the release.

## Configuration

// TODO(panosk)

## Contributing

// TODO(panosk)