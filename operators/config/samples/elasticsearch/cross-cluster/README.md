# CCR/CCS Samples
The file `remote.yml` contain the resources needed to test cross-cluster replication or cross-cluster search.

```bash
$ kubectl apply -f remote.yml
elasticsearch.elasticsearch.k8s.elastic.co/trust-one configured
elasticsearch.elasticsearch.k8s.elastic.co/trust-two configured
```

Two elasticsearch clusters should be created in the default namespace:

```bash
$ kubectl get elasticsearch,pods
NAME                                                   HEALTH    NODES   VERSION   PHASE         AGE
elasticsearch.elasticsearch.k8s.elastic.co/trust-one   unknown   1       6.6.2     Operational   2m
elasticsearch.elasticsearch.k8s.elastic.co/trust-two   unknown   1       6.6.2     Operational   2m

NAME                          READY   STATUS    RESTARTS   AGE
pod/trust-one-es-stc4kcmpp2   2/2     Running   0          2m
pod/trust-two-es-bldzf4wwpg   2/2     Running   0          2m
```

Check the status of the `RemoteCluster`:

```bash
$ kubectl get RemoteCluster/remotecluster-sample-1-2 -o yaml
apiVersion: elasticsearch.k8s.elastic.co/v1alpha1
kind: RemoteCluster
metadata:
[...]
  finalizers:
  - dynamic-watches.remotecluster.k8s.elastic.co
  generation: 1
  labels:
    controller-tools.k8s.io: "1.0"
    elasticsearch.k8s.elastic.co/cluster-name: trust-one
  name: remotecluster-sample-1-2
  namespace: default
[...]
spec:
  remote:
    k8sLocalRef:
      name: trust-two
      namespace: default
status:
  cluster-name: trust-one
  k8sLocal:
    remoteSelector:
      name: trust-two
      namespace: default
    remoteTrustRelationship: rcr-remotecluster-sample-1-2-default
  localTrustRelationship: rc-remotecluster-sample-1-2
  seedHosts:
  - trust-two-es-discovery.default.svc:9300
  state: Propagated
```

You can now use the string `remotecluster-sample-1-2` as a remote cluster identifier for cluster `trust-two` from cluster `trust-one`.
