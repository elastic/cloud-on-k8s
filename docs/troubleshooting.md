## Troubleshooting

### Pause the controllers

In case of trouble it might be useful to pause a controller for a particular resource.
It can be achieved by setting the annotation `common.k8s.elastic.co/pause` to `true` to any resource controlled by the operator :
- Stack
- ElasticsearchCluster
- Kibana

```yaml
metadata:
  annotations:
    common.k8s.elastic.co/pause: "true"
```

Or in one line:

```bash
$ kubectl annotate stack stack-sample --overwrite common.k8s.elastic.co/pause=true
```

Please note that if the annotation is set on the Stack all the dependents *(kibana, elasticsearchcluster)* are also paused.

### Debug logs

To enable debug logs for the operator, restart it with the flag `--log-level=DEBUG`. For example:

```shell
kubectl edit statefulset.apps -n elastic-system elastic-operator
```

and change the following lines from:

```yaml
    spec:
      containers:
      - args:
        - manager
        - --operator-roles
        - all
        - --log-level
        - INFO
```

to

```yaml
    spec:
      containers:
      - args:
        - manager
        - --operator-roles
        - all
        - --log-level
        - DEBUG
```
