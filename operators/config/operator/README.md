# Operator deployment resources

The Elastic Operator can be deployed in 2 different modes using the following arguments:

* `--operator-roles`: namespace, global, webhook or all
* `--operator-namespace`: namespace the operator runs in
* `--namespace`: namespace in which resources should be watched (defaults to all namespaces)

## Deployment mode

### All-in-one

A single operator with all roles, that manages resources in all namespaces.

```bash
OPERATOR_IMAGE=<?> NAMESPACE=<?> make all-in-one \
	kubectl apply -f -
```

### Global and Namespace

One global operator for high-level cross-cluster features and one namespace operator per namespace.

#### Global

A global operator with the webhook server (optional) that acts across namespaces and is not related to a specific deployment of the Elastic stack.
The global operator deployed cluster-wide is responsible for high-level cross-cluster features (CCR, CCS, enterprise licenses).

```bash
OPERATOR_IMAGE=<?> NAMESPACE=<?> make global \
	kubectl apply -f -
```

#### Namespace

A namespace operator that manages resources in a given namespace.

```bash
OPERATOR_IMAGE=<?> NAMESPACE=<?> MANAGED_NAMESPACE=<?> make namespace \
	kubectl apply -f -
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