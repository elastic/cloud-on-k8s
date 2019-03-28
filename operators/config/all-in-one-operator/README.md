# Single namespace operator resources

The following yaml files can be customized as required:

## namespace.yaml

Creates a namespace for the namespace operator to be deployed into. Name can be changed to the desired namespace.

## service_account.yaml

Service account used by the operator to reach the k8s api. Must match the service account used in `operator.yaml`.

## operator.yaml

Describes the operator stateful set and service.
Namespace can be changed according to what is set in `namespace.yaml`.
Service accounts can be changed according to what is set in `service_account.yaml`.

Namespace operator args can be customized:

* `--namespace`: namespace in which resources should be watched (defaults to "default")

## role.yaml

Describes permissions for several api calls the operator needs to perform.

## role_bindings.yaml

Allows the operator to perform calls described in `cluster_role.yaml` in the specified namespace ("default"). The namespace can be changed, according to the `--namespace` argument of the operator in `operator.yaml`.
