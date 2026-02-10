# 5. Namespace Filtering with Label Selectors

## Context and Problem Statement

In Kubernetes, it is often necessary to limit the scope of an operator to a subset of namespaces. The ECK operator previously supported only two modes: cluster-wide (all namespaces) or a static list of namespaces. Both approaches have limitations in dynamic environments with frequent namespace changes.

## Considered Options

### Option 1: Dynamically update --namespaces flag

Run the operator with the `--namespaces` flag, updating it dynamically to include only the desired namespaces.

**Drawback:** Not maintainable in dynamic environments with frequent namespace changes.

### Option 2: One operator per namespace

Deploy a separate operator instance for each namespace to achieve strict separation.

**Drawback:** Higher resource usage and operational overhead, with little benefit for most use cases.

### Option 3: Use a label selector (chosen)

Extend the operator to support a label selector for namespaces. The operator manages only namespaces matching the selector.

**Advantage:** Flexible, scalable, and easy to maintain in dynamic environments.

### Option 4: One operator per cluster
Deploy a separate Kubernetes cluster for each environment, with one operator per cluster. This provides strong isolation between environments.

**Drawbacks:**
- Significantly higher resource usage and operational overhead.
- Elasticsearch clusters must be exposed via ingress or other networking solutions for cross-cluster communication, increasing complexity and potential security risks.

## Decision Outcome

Option 3 was chosen. The ECK operator now supports namespace filtering using label selectors. This allows multiple ECK operators to manage different sets of namespaces in the same Kubernetes cluster.

## How It Works

The ECK operator uses Kubernetes label selectors to filter namespaces. A label selector specifies which labels must match. Only namespaces with the required labels are managed by the operator.

For example, with a label selector `environment: production`, the operator will only manage namespaces labeled with `environment: production`.

## Benefits

- **Targeted Management**: The operator can be limited to specific namespaces, useful in shared clusters.
- **Resource Optimization**: Limiting the operator's scope improves resource usage and reduces conflicts.
- **Security and Isolation**: Namespace filtering helps enforce isolation between applications or teams.

## Configuration Example

To enable namespace filtering, start the ECK operator with the `--namespace-label-selector` flag. For example:

```sh
manager --namespace-label-selector=environment=production
```

This instructs the operator to manage only those namespaces that have the label `environment=production`.

Example namespace manifest:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
  labels:
    environment: production
```

Any resources (such as Elasticsearch, Kibana, etc.) in `my-namespace` will only be managed by the operator if the namespace itself has the correct label.

## Conclusion

Namespace filtering with label selectors provides a flexible way to control the scope of the ECK operator. This enables efficient management of multiple namespaces and improves isolation and security.
