# Quickstart

## Overview

You will learn how to:

* Deploy the operator in your Kubernetes cluster
* Deploy an Elasticsearch cluster
* Deploy a Kibana instance
* Access Elasticsearch and Kibana
* Deep dive with:
    * Securing your cluster
    * Using persistent storage
    * Additional features

## Requirements

* Kubernetes 1.11+

## Deploy the operator

1. Install [custom resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), to extend the apiserver with additional resources:

```bash
# TODO: fix the URL once the repo is public (for now it does not work without a secret token query param)
kubectl apply -f https://raw.githubusercontent.com/elastic/k8s-operators/master/operators/config/crds.yaml

```

2. Install the operator with its RBAC rules:

```bash
# TODO: reference a public url to a single hardcoded version of the yaml file here instead
NAMESPACE=elastic-operator kubectl apply -f config/operator/all-in-one.yaml
```

3. Monitor the operator logs:

```bash
kubectl -n elastic-operator logs -f statefulset.apps/elastic-operator
```

## Deploy Elasticsearch

Let's apply a simple Elasticsearch cluster specification, with one node:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: sample
spec:
  version: 7.0.0
  nodes:
  - nodeCount: 1
    config:
      node.master: true
      node.data: true
      node.ingest: true
EOF
```

The operator will automatically take care of managing pods and resources corresponding to the desired cluster. It may take up to a few minutes until the cluster is ready.

### Monitor cluster health and creation progress

Get an overview of current Elasticsearch clusters in the Kubernetes cluster, including their health, version and number of nodes:

```bash
kubectl get elasticsearch
```

While the cluster is being created, you might notice there is no "health" yet and the phase is still "Pending". After a while the cluster should appear as "Running", with a green health.

You should notice one Pod in the process of being started:

```bash
kubectl get pods --selector='elasticsearch.k8s.elastic.co/cluster-name=sample'
```

### Access Elasticsearch

A ClusterIP Service is automatically created for your cluster:

```
kubectl get service sample-es
```

Elasticsearch can be accessed from the Kubernetes cluster, using the URL `http://sample-es:9200`.

Use `kubectl port-forward` to access Elasticsearch from your local workstation:

```bash
kubectl port-forward service/sample-es 9200
```

Then in another shell:

```bash
curl "localhost:9200/_cat/health?v"
```

## Deploy Kibana

### Target our sample Elasticsearch cluster

Specify a Kibana instance and associate it with your sample Elasticsearch cluster:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kibana.k8s.elastic.co/v1alpha1
kind: Kibana
metadata:
  name: sample
spec:
  version: 6.4.2
  nodeCount: 1
---
apiVersion: associations.k8s.elastic.co/v1alpha1
kind: KibanaElasticsearchAssociation
metadata:
  name: kibana-es-sample
spec:
  elasticsearch:
    name: sample
    namespace: default
  kibana:
    name: sample
    namespace: default
EOF
```

### Monitor Kibana health and creation progress

Similar to Elasticsearch, you can retrieve some details about Kibana instances:

```bash
kubectl get kibana
```

And the associated Pods:

```bash
kubectl get pod --selector='kibana.k8s.elastic.co/name=sample'
```

### Access Kibana

A `ClusterIP` Service was automatically created for Kibana:

```
kubectl get service sample-kibana
```

Use `kubectl port-forward` to access Kibana from your local workstation:

```bash
kubectl port-forward service/sample-kibana 5601
```

You can then open http://localhost:5601 in your browser.

## Upgrade your deployment

We can easily apply any modification to the original cluster specification. The operator makes sure that changes are applied to the existing cluster, avoiding downtime.

Grow the cluster to 3 nodes:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: sample
spec:
  version: 7.7.0
  nodes:
  - nodeCount: 3
    config:
      node.master: true
      node.data: true
      node.ingest: true
EOF
```

## Going further

### Securing your cluster

To secure your production-grade Elasticsearch deployment, you can:

* Use XPack security for encryption and authentication (TODO: link here to a tutorial on how to manipulate certs and auth)
* Set up an ingress proxy layer (TODO: link here to the nginx ingress sample)

### Using persistent storage

The sample cluster you have just deployed uses an [emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir), which may not qualify for production workloads. 

You can request a PersistentVolumeClaim in the cluster specification, to target any PersistentVolume class available in your Kubernetes cluster:

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: my-cluster
spec:
  version: "7.7.0"
  nodes:
  - nodeCount: 3
    config:
      node.master: true
      node.data: true
      node.ingest: true
    volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes:
        - ReadWriteOnce
        resources:
          requests:
            storage: 100GB
        storageClassName: gcePersistentDisk # can be any available storage class
```

To aim for the best performance, the operator supports persistent volumes local to each node. For more details, see:
 
 * [elastic local volume dynamic provisioner](https://github.com/elastic/k8s-operators/tree/master/local-volume) to setup dynamic local volumes based on LVM
 * [kubernetes-sigs local volume static provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner) to setup static local volumes
 
### Additional features

The operator supports the following features:

* Node-to-node TLS encryption
* User management
* Automated snapshots
* Nodes resources limitations (CPU, RAM, disk)
* Cluster update strategies
* Version upgrades
* Node attributes
* Cross-cluster search and replication
* Licensing
* Operator namespace management
* APM server deployments
* Pausing reconciliations

TODO: add a link to the homepage of all features documentation.
