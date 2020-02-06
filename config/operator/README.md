# Operator deployment resources

The Elastic Operator can be deployed in 2 different modes. Either watching all namespaces or a subset of namepspaces with restricted RBAC permissions. Furthermore it is possible, even if not recommended, to deploy the operator into the same namespace as the workloads it is managing:

* `--operator-namespace`: namespace the operator runs in
* `--namespaces`: comma-separated list of namespaces in which resources should be watched (defaults to all namespaces)


## Deployment mode

### All-in-one

A single operator with all roles, that manages resources in all namespaces.

```bash
OPERATOR_IMAGE=<?> NAMESPACE=<?> make generate-all-in-one | kubectl apply -f -
```

### Namespaced

One or more operators managing resources in a given set of of namespaces.

```bash
OPERATOR_IMAGE=<?> NAMESPACE=<?> MANAGED_NAMESPACES=<?> make generate-namespace | kubectl apply -f -
```

## Role of each YAML file

### namespace.yaml

Describes the namespace for the operator to be deployed into.

### operator.yaml

Describes the operator StatefulSet.

### service_account.yaml

Service account used by the operator to reach the Kubernetes api.
Must match the service account used in `operator.yaml`.
Set the flags to chose the operator roles and namespace in which resources should be watched.

### cluster_role.yaml

Describes permissions for several api calls the operator needs to perform.

### role_bindings.yaml or cluster_role_bindings.yaml

Allows the operator to perform calls described in `cluster_role.yaml` in the operator namespace.
