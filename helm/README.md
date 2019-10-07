# Elastic Cloud on Kubernetes Helm Chart

This functionality is in beta and is subject to change. The design and code is less mature than official GA features and is being provided as-is with no warranties. Beta features are not subject to the support SLA of official GA features.

## Requirements

* [Helm](https://helm.sh/) >= 2.8.0
* Kubernetes >= 1.8

## Installing

* Add the elastic helm charts repo
  ```
  helm repo add elastic https://helm.elastic.co
  ```
* Install it
  ```
  helm install --name elastic-cloud elastic/elastic-cloud --version 0.9.0
  ```

## Configuration

### Elasticsearch

| Parameter                  | Description                                                                                                                                                                                                                                                                                                                | Default                                                                                                                   |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `version`              | The Elasticsearch version to use | `7.1.0` |
| `nodeCount`            | How many pods to create          | `1`     |

### Kibana

| Parameter                  | Description                                                                                                                                                                                                                                                                                                                | Default                                                                                                                   |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `version`              | The Kibana version to use | `7.1.0` |
| `nodeCount`            | How many pods to create          | `1`     |

## Try it out

In [examples/](./examples) you will find some example configurations. These examples are used for the automated testing of this helm chart

### Nginx Ingress

To deploy a cluster with all default values and ingresses

```
cd examples/nginx.ingress
make
```
