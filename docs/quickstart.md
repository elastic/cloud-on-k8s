# Quickstart

## Overview

You will learn how to:

* Deploy the operator in your Kubernetes cluster
* Deploy the Elasticsearch cluster
* Deploy the Kibana instance
* Upgrade your deployment
* Deep dive with:
    * Secure your cluster
    * Use persistent storage
    * Additional features

## Requirements

* Kubernetes 1.11+

## Deploy the operator in your Kubernetes cluster

1. Install [custom resource definitions](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), to extend the apiserver with additional resources:

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/elastic/k8s-operators/master/operators/config/crds.yaml
    ```

2. Install the operator with its RBAC rules:

    ```bash
    kubectl apply -f https://raw.githubusercontent.com/elastic/k8s-operators/master/operators/config/all-in-one.yaml
    ```

3. Monitor the operator logs:

    ```bash
    kubectl -n elastic-system logs -f statefulset.apps/elastic-operator
    ```

## Deploy the Elasticsearch cluster

Let's apply a simple Elasticsearch cluster specification, with one node:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: quickstart
spec:
  version: 7.0.0
  nodes:
  - nodeCount: 1
    config:
      node.master: true
      node.data: true
      node.ingest: true
      xpack.license.self_generated.type: trial
EOF
```

The operator will automatically take care of managing Pods and resources corresponding to the desired cluster. It may take up to a few minutes until the cluster is ready.

### Monitor cluster health and creation progress

Get an overview of the current Elasticsearch clusters in the Kubernetes cluster, including their health, version and number of nodes:

```bash
kubectl get elasticsearch
```
```
NAME      HEALTH    NODES     VERSION   PHASE         AGE
quickstart    green     1         7.0.0     Operational   1m
```

When you create the cluster, there is no `HEALTH` status and the `PHASE` is `Pending`. After a while, the `PHASE` turns into `Operational`, and `HEALTH` becomes `green`.

You can see that one Pod is in the process of being started:

```bash
kubectl get pods --selector='elasticsearch.k8s.elastic.co/cluster-name=quickstart'
```
```
NAME                   READY     STATUS    RESTARTS   AGE
quickstart-es-5zctxpn8nd   1/1       Running   0          1m
```

And access the logs for that Pod:

```bash
kubectl logs -f quickstart-es-5zctxpn8nd
```

### Access Elasticsearch

#### ClusterIP service

A ClusterIP Service is automatically created for your cluster:

```bash
kubectl get service quickstart-es
```
```
NAME        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
quickstart-es   ClusterIP   10.15.251.145   <none>        9200/TCP   34m
```

You can access Elasticsearch from the Kubernetes cluster, using the URL `https://quickstart-es:9200`.

#### Retrieve credentials

A default user named `elastic` was automatically created. Its password is stored as a Kubernetes secret:

```bash
PASSWORD=$(kubectl get secret quickstart-elastic-user -o=jsonpath='{.data.elastic}' | base64 --decode)
```

#### Request Elasticsearch

1. Use `kubectl port-forward` to access Elasticsearch from your local workstation:

    ```bash
    kubectl port-forward service/quickstart-es 9200
    ```

2. In another shell, request the Elasticsearch endpoint (skipping the certificate verification for now):

    ```bash
    curl -u "elastic:$PASSWORD" -k "https://localhost:9200"
    ```
    ```
    {
      "name" : "quickstart-es-5zctxpn8nd",
      "cluster_name" : "quickstart",
      "cluster_uuid" : "2sUV1IUEQ5SA5ZSkhznCHA",
      "version" : {
        "number" : "7.0.0",
        "build_flavor" : "default",
        "build_type" : "docker",
        "build_hash" : "b7e28a7",
        "build_date" : "2019-04-05T22:55:32.697037Z",
        "build_snapshot" : false,
        "lucene_version" : "8.0.0",
        "minimum_wire_compatibility_version" : "6.7.0",
        "minimum_index_compatibility_version" : "6.0.0-beta1"
      },
      "tagline" : "You Know, for Search"
    }
    ```

## Deploy the Kibana instance

### Target the Elasticsearch cluster

Specify a Kibana instance and associate it with your quickstart Elasticsearch cluster:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kibana.k8s.elastic.co/v1alpha1
kind: Kibana
metadata:
  name: quickstart
spec:
  version: 7.0.0
  nodeCount: 1
---
apiVersion: associations.k8s.elastic.co/v1alpha1
kind: KibanaElasticsearchAssociation
metadata:
  name: kibana-es-quickstart
spec:
  elasticsearch:
    name: quickstart
    namespace: default
  kibana:
    name: quickstart
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
kubectl get pod --selector='kibana.k8s.elastic.co/name=quickstart'
```

### Access Kibana

A `ClusterIP` Service was automatically created for Kibana:

```
kubectl get service quickstart-kibana
```

Use `kubectl port-forward` to access Kibana from your local workstation:

```bash
kubectl port-forward service/quickstart-kibana 5601
```

You can then open http://localhost:5601 in your browser.

## Modify your deployment

You can apply any modification to the original cluster specification. The operator makes sure that changes are applied to the existing cluster, avoiding downtime.

Grow the cluster to 3 nodes:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: quickstart
spec:
  version: 7.0.0
  nodes:
  - nodeCount: 3
    config:
      node.master: true
      node.data: true
      node.ingest: true
      xpack.license.self_generated.type: trial
EOF
```

## Deep dive

### Secure your cluster

To secure your production-grade Elasticsearch deployment, you can:

* Use XPack security for encryption and authentication (TODO: link here to a tutorial on how to manipulate certs and auth)
* Set up an ingress proxy layer ([example using NGINX](https://github.com/elastic/cloud-on-k8s/blob/master/operators/config/samples/ingress/nginx-ingress.yaml))

### Use persistent storage

The quickstart cluster you have just deployed uses an [emptyDir volume](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir), which may not qualify for production workloads. 

You can request a PersistentVolumeClaim in the cluster specification, to target any PersistentVolume class available in your Kubernetes cluster:

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: Elasticsearch
metadata:
  name: my-cluster
spec:
  version: 7.0.0
  nodes:
  - nodeCount: 3
    config:
      node.master: true
      node.data: true
      node.ingest: true
      xpack.license.self_generated.type: trial
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
 
 * [elastic local volume dynamic provisioner](https://github.com/elastic/cloud-on-k8s/tree/master/local-volume) to setup dynamic local volumes based on LVM
 * [kubernetes-sigs local volume static provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner) to setup static local volumes
 
### Additional features

The operator supports the following features:

* Node-to-node TLS encryption
* User management
* Secure settings (for eg. automated snapshots)
* Nodes resources limitations (CPU, RAM, disk)
* Cluster update strategies
* Version upgrades
* Node attributes
* Cross-cluster search and replication
* Licensing
* Operator namespace management
* APM server deployments
* Pausing reconciliations
* Full cluster restart
