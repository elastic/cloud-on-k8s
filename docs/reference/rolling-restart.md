---
navigation_title: Elasticsearch rolling restart
applies_to:
  deployment:
    eck: all
---

# Elasticsearch rolling restart

You can trigger a graceful rolling restart of an {{es}} cluster without changing the cluster spec (version, image, or pod template). The operator reuses the same rolling upgrade path: it uses the Elasticsearch node shutdown API, respects the same [upgrade predicates](./upgrade-predicates.md), and restarts one node at a time.

## Annotations

Set these annotations on the Elasticsearch resource:

| Annotation | Description |
|------------|-------------|
| `eck.k8s.elastic.co/restart-trigger` | **Required to trigger.** Set or change this value (for example to a timestamp) to start a rolling restart. The value is propagated to pod annotations and is visible in Elasticsearch's node shutdown API as the shutdown reason. |
| `eck.k8s.elastic.co/restart-allocation-delay` | Optional. Duration string (for example `"5m"`, `"20m"`) used as `allocation_delay` for the node shutdown API during rolling restarts and upgrades. If unset, Elasticsearch's default (for example 5m) is used. Invalid values or negative values, are logged and ignored. |

To trigger another rolling restart later, update the `restart-trigger` value (for example to a new timestamp). Removing the annotation does **not** trigger a new restart; the operator retains the last trigger value on the pod template.

## Example

```yaml
apiVersion: elasticsearch.k8s.elastic.co/v1
kind: Elasticsearch
metadata:
  name: my-cluster
  annotations:
    eck.k8s.elastic.co/restart-trigger: "2026-01-14T12:00:00Z"
    eck.k8s.elastic.co/restart-allocation-delay: "20m"
spec:
  version: 8.15.0
  nodeSets:
    - name: default
      count: 3
      config:
        node.store.allow_mmap: false
```

Progress is visible in the Elasticsearch resource status under **In Progress Operations** → **Upgrade**, with node-level messages such as "Deleting pod for rolling restart".
