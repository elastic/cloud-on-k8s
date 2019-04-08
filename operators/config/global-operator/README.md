# Global operator resources

The following yaml files can be customized as required:

## kustomization.yaml

Defines the namespace in which the global operator will be deployed. Can be changed as required.

## namespace.yaml

Creates a namespace for the global operator to be deployed into. Must match the namespace set in `kustomization.yaml`.

## service_account.yaml

Service account used by the operator to reach the k8s api. Must match the service account used in `operator.yaml`.

## operator.yaml

Describes the operator stateful set and service.
Service accounts can be changed according to what is set in `service_account.yaml`.

Global operator args can be customized:

* `--operator-roles`: should be `global` and/or `webhook` if the webhook server should be deployed as well
* `--namespace`: namespace in which resources should be watched (defaults to all namespaces)

## cluster_role.yaml

Describes permissions for several api calls.

## cluster_role_bindings.yaml

Allows the operator to perform calls described in `cluster_role.yaml` across all namespaces. Can be changed to a more restricted "role bindings" limited to a single namespace if required.
