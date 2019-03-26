# Quickstart tutorial

## Overview

This quickstart tutorial will guide through:

* Deploying the operator in your Kubernetes cluster
* Deploying an Elasticsearch cluster
* Deploying a Kibana instance
* Upgrading your deployment

## Requirements

* Kubernetes 1.11+
* kustomize 1.0.9+

## Deploy the operator

### Install CRDs

Install [custom resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), to extend the apiserver with additional resources:

```
kubectl apply -f config/crds
```

### Deploy the operator

TODO: use the official operator image once built, and get rid of kustomize.

To install the operator along with its RBAC rules, run:

```
export OPERATOR_IMAGE=TODOFIXME
kustomize build config/global-operator | sed -e "s|\$OPERATOR_IMAGE|$OPERATOR_IMAGE|g" | kubectl apply -f -
```

### Monitor the operator logs

To get some insights about current reconciliation loop iterations, run:

```
kubectl --namespace=elastic-system logs -f statefulset.apps/elastic-global-operator
```

## Deploy Elasticsearch and Kibana

### 3-nodes cluster sample

A sample cluster definition [is provided](../../operators/config/samples/es_kibana_sample.yaml) in the samples directory. It describes a 3 nodes cluster, associated to a Kibana instance. The cluster endpoint is exposed through a LoadBalancer.

*Important:* this sample cluster does not rely on PersistentVolumes. Consider it for demonstration purpose only. For more details on setting-up production grade clusters with reliable storage, see TODO: doc on storage here.

Let's deploy it:

```
kubectl apply -f config/samples/es_kibana_sample.yaml
```

The operator will take care of managing a set of pods and resources corresponding to the desired cluster. Deployment to a running state can take up to a few minutes.

### Retrieve cluster details

Get an overview of current Elasticsearch clusters in the Kubernetes cluster, including their health, version and number of nodes:

```
kubectl get elasticsearch
```

The same command is available for Kibana:

````
kubectl get kibana
````

### Access the Elasticsearch endpoint

Once the cluster is ready, you can access it through its public endpoint, as created by the LoadBalancer service.

```
kubectl get service elasticsearch-sample-es
```

Notice the `external-ip` column, corresponding to the public endpoint. You can retrieve the IP only:

```
export PUBLIC_IP=$(kubectl get service elasticsearch-sample-es -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
```

Since the sample is using a `Trial` license configured with XPack security, the cluster is using TLS and user authentication.

A sample user `elastic` was created automatically. To retrieve its password:

```
export PASSWORD=$(kubectl get secret elasticsearch-sample-elastic-user -o jsonpath='{.data.elastic}' | base64 -D)
```

You can then attempt a request to the Elasticsearch endpoint:

```
curl -k -u elastic:$PASSWORD https://$PUBLIC_IP:9200
```

Notice how the request above is ignoring TLS certificates (`-k`).
A Certificate Authority was setup for this cluster, that you can retrieve and use to validate the server identity:

```
kubectl get secret elasticsearch-sample-ca
```

### Access Kibana

Retrieve Kibana public IP:

```
kubectl get service kibana-sample-kibana
```

You can then access it from your browser, by using port 5601.
