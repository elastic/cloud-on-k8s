# Store the last applied pod specification to the PVCs

* Status: proposed
* Deciders: [list everyone involved in the decision] <!-- optional -->
* Date: [YYYY-MM-DD when the decision was last updated] <!-- optional -->

Technical Story: TBD

## Context and Problem Statement

In some edge cases the cluster specification may evolve while there is no pod running because they all have been deleted.
It can be the case if the controller is not running, if there is a network partition between the controller and the K8S API
or for any other reason that prevents the Elastic controller to create some Pods. If in the meantime a human operator has updated
the spec of the cluster with an incompatible change, like a major ES version bump, we could be in a situation where we can't
reuse the PVCs.

## Decision Drivers

* The state of a cluster lies inside the PVCs, nowhere else, we should be able to restore the complete state of a cluster from them.
* We must not lose some data, give up a PVC because the reconciliation loop doesn't know what to do with it.
* We must handle some edge cases as the one described above.
* The design should be eventually compatible with an inline update strategy, e.g. : some of your pods can't be started because they do not have enough memory
* If a `PVC` remains orphaned after some reconciliation loops it could be useful for the support team :
    * to have the last Pod specification that has been applied to the PVC
    * to be able to manually edit the config a specific node and resume the controller

## Considered Options

* Store the last applied pod specification to the PVCs

The last `PodSpecContext` applied by the controller could be stored as an annotation on the PVC. It is already done by K8S
on some objects _(but not for the same purpose I think)_ :

```yaml
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1"
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"annotations":{},"name":"nginx-deployment","namespace":"default"},
      "spec":{"replicas":2,"selector":{"matchLabels":{"app":"nginx"}},"template":{"metadata":{"labels":{"app":"nginx"}},
      "spec":{"containers":[{"image":"nginx:1.7.9","name":"nginx","ports":[{"containerPort":80}]}]}}}}
[...]
```

We can use some annotations in a similar way to store the last Pod specification applied to a PVC. It could be used to
safely restart a Pod even if the `ElasticsearchCluster` object has been updated with some incompatible changes.
In this last case we can restore the cluster from its last state and use the best strategy to update it according to changes
that have been made to the `ElasticsearchCluster` object.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  annotations:
    control-plane.alpha.kubernetes.io/leader: '{"holderIdentity":"a2e97f7d-2573-11e9-82e4-080027545704","leaseDurationSeconds":15,...}'
    elastic.co/last-applied-spec: '{"PodSpec":{ ... "image":"docker.elastic.co/elasticsearch/elasticsearch:6.4.2" ... }}'
```

* _I don't have an option 2 to reliably restore a cluster from a "cold start" and having some incompatible changes in the desired state_

## Decision Outcome

Chosen option: "[option 1]", because [justification. e.g., only option, which meets k.o. criterion decision driver | which resolves force force | … | comes out best (see below)].

### Positive Consequences <!-- optional -->

* [e.g., improvement of quality attribute satisfaction, follow-up decisions required, …]
* …

### Negative Consequences <!-- optional -->

* [e.g., compromising quality attribute, follow-up decisions required, …]
* …

## Pros and Cons of the Options <!-- optional -->

### [option 1]

[example | description | pointer to more information | …] <!-- optional -->

* Good, because [argument a]
* Good, because [argument b]
* Bad, because [argument c]
* … <!-- numbers of pros and cons can vary -->

### [option 2]

[example | description | pointer to more information | …] <!-- optional -->

* Good, because [argument a]
* Good, because [argument b]
* Bad, because [argument c]
* … <!-- numbers of pros and cons can vary -->

### [option 3]

[example | description | pointer to more information | …] <!-- optional -->

* Good, because [argument a]
* Good, because [argument b]
* Bad, because [argument c]
* … <!-- numbers of pros and cons can vary -->

## Links <!-- optional -->

* [Link type] [Link to ADR] <!-- example: Refined by [ADR-0005](0005-example.md) -->
* … <!-- numbers of links can vary -->
