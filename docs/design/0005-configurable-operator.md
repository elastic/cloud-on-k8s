# 5. Configurable operator and RBAC permissions

* Status: proposed
* Deciders: cloud-on-k8s team
* Date: 2019-02-13

## Context and Problem Statement

Most operators out there operate in one of these two modes:

1. Cluster-wide operator. Can manage resources in all namespaces, with cluster-wide admin privilege. A single operator running on the cluster.
2. Namespaced operator. Can manage resources in the namespace it's deployed in, with admin permissions in that namespace. Several operators can be running in different namespaces.

The first option (cluster-wide single operator) has some major drawbacks:

* it does not scale well with the number of clusters
* it requires elevated permissions on the cluster

The second option (namespace operator) also has some major drawbacks:

* it does not play well with cross-namespace features (a single enterprise license pool for multiple clusters in multiple namespaces, cross-cluster search and replication on clusters across namespaces)
* to deploy 5 clusters in 5 different namespaces, it requires 5 operators running. A single one could have been technically enough.


## Decision Drivers

* Scalability (down): must be able to scale down to single-cluster deployments without being overly complex to manage.
* Scalability (up): Must be able to scale up with large k8s installations and manage tens of thousands of clusters simultaneously.

  * In any sufficiently large installation with clusters under load there is going to be high variance between response times for different ES API calls, and one clusters responsiveness should not be able to negatively affect the operations of any other cluster.

* Security: The solution should have an easy to understand story around credentials, RBAC permissions and service accounts.

  * As far as possible, adhere to the principle of least amount of access: we should not require more permissions than strictly necessary for the operators to accomplish what they need to.

## Considered Options

### Option 1: global and namespace operators

[In a previous design proposal](https://github.com/elastic/cloud-on-k8s/blob/main/docs/design/0002-global-operator/0002-global-operator.md), we introduced the concepts of one global and several namespace operators.

The global operator deployed cluster-wide responsible for high-level cross-cluster features (CCR, CCS, enterprise licenses).
Namespace operators are responsible for managing clusters in a single namespace. There might be several namespace operators running on a single cluster.
Namespace operators would potentially be automatically deployed by the global operator.
The doc also introduces an "hybrid" approach, where the global operator can be configured to manage Elasticsearch clusters cluster-wide.

### Option 2: configurable operator in terms of namespaces and roles

A "single" operator concept (no "global" vs. "namespace" terminology), which can take several configuration options:

* list of namespaces in which it should manage resources
  * can be a single namespace: `["default"]` (requires RBAC permissions on a single namespace)
  * can be a list of namespaces: `["default", "production", "team-X"]` (requires RBAC permissions on a list of namespaces)
  * can be all namespaces (requires cluster-level permissions)
* list of roles, specifying the controllers it should run internally
  * can be oriented towards ES clusters management: `["elasticsearch", "kibana"]`
  * can be oriented towards licensing management: `["licensing"]`
  * can be oriented towards cross-cluster features: `["cross-cluster"]`
  * can be everything

According to the list of namespaces and roles, the operator can be deployed with the strict-minimum RBAC permissions. It gives some flexibility around running a single or multiple operators in a single Kubernetes cluster, with different scopes according to one's needs.

Because of the increased complexity in mapping the exact RBAC permissions and deployment options in yaml resources, it would require some deployment tooling. Introducing the `elastic-operator` cli (name and exact specifications TBD):

```bash
# cli sample usage (exact specification TBD)
> ./elastic-operator generate --config=operators.yaml
# generates one or multiple yaml files ready-to-deploy, including Deployment, ServiceAccount, (Cluster)Roles, (Cluster)RoleBindings.
```

```yaml
# sample configuration (exact specification TBD)
operators:
    # manage clusters in namespace "team-A-production"
    - image: docker.elastic.co/k8s-operators:1.0
      roles: ["elasticsearch", "kibana"]
      managedNamespaces: ["team-A-production"]
      namespace: "team-A-production"
    # manage clusters in namespace "team-A-development"
    - image: docker.elastic.co/k8s-operators:1.1-alpha1
      roles: ["elasticsearch", "kibana"]
      managedNamespaces: ["team-A-development"]
      namespace: "team-A-development"
    # manage enterprise licenses in all namespaces
    - image: docker.elastic.co/k8s-operators:1.0
      roles: ["licensing"]
      clusterWide: true
      namespace: "elastic-system"
    # manage CCS/CCR in namespaces "team-A-production" and "team-B-production"
    - image: docker.elastic.co/k8s-operators:1.0
      roles: ["cross-cluster"]
      managedNamespaces: ["team-A-production", "team-B-production"]
      namespace: "elastic-system"
```

Compatibility of this configuration with other tools such as Helm remains to be evaluated.

The CLI would perform simple checks around configuration consistency. For example, it would reject a configuration where 2 operators would concurrently manage Elasticsearch clusters in the same namespace.

This is not enough to prevent a user from accidentally deploy conflicting operators. We also need to enforce some locking/leader election mechanism at the operator-level. Thanks to the reconciliation-loop approach, it's probably fine to have 2 operators running concurrently over a limited short timespan. Hence, it's probably OK for the lock mechanism to be optimistic.

So far, the [controller-runtime cannot watch resources in a list of namespaces](https://github.com/kubernetes-sigs/controller-runtime/issues/218). It's either one namespace, or all namespaces. It seems planned for the long-term though, and [workarounds can exist](https://github.com/operator-framework/operator-sdk/issues/767).

The idea of having one operator deploying other operators automatically is out of scope for this option, but there is nothing preventing it from being applied there (kiss implementation: run the CLI periodically).

## Decision Outcome

Chosen option: option 2 (configurable operator), because it gives us more flexibility on the deployment strategy, and allows restricting RBAC permissions to a finer-grained level.

### Positive Consequences

* Much more flexibility to cover various deployment scenarios
  * a single cluster-wide operator
  * one operator per namespace
  * one operator for all production namespaces, another one for all staging namespaces
  * and so on
* We don't have to require cluster-level permissions to handle enterprise licensing
* A single operator concept, no namespace/global/ecosystem vocabulary madness

### Negative Consequences

* Too many options can lead to confusion, we need proper documentation
* Increased yaml complexity: need to develop a tool to generate yaml specifications
* The controller-runtime is not ready yet for multi-namespace watches

## Pros and Cons of the Options

### Option 1 (global and namespace operators)

Pros:

* "Only" 2 concepts to understand
* Seems to match most use cases

Cons:

* Need for a "hybrid" version to deploy everything in a single restricted namespace (for ex. default), which is a bit confusing
* The global-operator needs elevated permissions on the cluster (for ex. read access to all secrets)
* One might want to use several global-operators on a single cluster
* One operator per namespace might lead to deploy too many operators

Introducing the multi-namespace feature from Option 2 here would solve several concerns.

### Option 2 (configurable operator)

Pros:

* Flexibility for any deployment setup
  * a single cluster-wide operator
  * one operator per namespace
  * one operator for all production namespaces, another one for all staging namespaces
  * and so on
* Restricted RBAC permissions according to one's exact needs
* A "single" operator simplifies communication around the project
* Actually a superset of option 1, the concept of global + namespaces operators can be easily implemented

Cons:

* Need to develop extra tooling
* Need some lock/leader-election protection against inconsistent configurations
* Increased complexity in the number of available options
* The controller-runtime is not ready yet for multi-namespace watches
* Managing many namespaces is rather "manual": adding a new namespace might require adding a new configuration item, unless using cluster-wide operators

## Links

* [Discussion issue](https://github.com/elastic/cloud-on-k8s/issues/374)
* [Global operator ADR](https://github.com/elastic/cloud-on-k8s/blob/main/docs/design/0002-global-operator/0002-global-operator.md)
