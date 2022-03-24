# 8. Volume Management in case of disruption

**Update (2019-07-30)**:
We decided to rely on StatefulSets to manage PersistentVolumes. While this ADR remains valid, our range of action is now limited to what is supported by the StatefulSet controller, responsible for creating and reusing PVCs.

* Status: proposed
* Deciders: cloud-on-k8s team
* Date: 2019-03-08

## Context and Problem Statement

The aim of this document is to capture some scenarios where a pvc gets orphaned and define how the “reuse pvc” mechanism must behave.
This document does not deal with the reuse of a PVC after the spec of the cluster has been updated _(that is, "inline" VS "grow-and-shrink" updates)_.
It is a complex scenario which deserves its own ADR.


As a preamble before we dive into the different use-cases and scenarios here are some considerations about what can lead
to a disruption and a reminder about some constraints raised by storage classes.

### Disruptions
A Pod does not disappear until a person or the controller deletes it or there is an unavoidable hardware or system software error.
The reasons can be classified into 2 main categories:

* There is an **external involuntary** disruption:
  * Hardware failure
  * VM instance is deleted
  * Kernel panic
  * Any runtime panic (for example `containerd` crash)
  * Eviction, but it is not supposed to happen as long as we use a [QoS class of Guaranteed](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/#create-a-pod-that-gets-assigned-a-qos-class-of-guaranteed)
* There is an **external voluntary** disruption
  * The node hosting the Pod is drained because, some _(non exhaustive)_ examples are:
    * The K8S node is about to be upgraded or repaired
    * The K8S cluster is scaling down
  * The pod is manually deleted by someone (not only as in error but also because sometimes a reboot can fix a problem)

### Storage class constraints
Storage classes do not all provide the same capabilities when it comes to reusing a volume, for instance:

* Google persistent disks can be attached from a single availability zone
* Regional persistent disks replicate the data between 2 zones in the same region
* A volume backed by our elastic-local storage class can only be reused on the same node

At this stage it is worth mentioning that even if the K8S scheduler uses some predicates to reschedule a pod on a node
where the volume can be reused or attached, it does not preserve the capacity needed to reschedule the pod.
For instance if a pod was using a local volume and if the node runs out of capacity while the pod is being recreated
then it becomes impossible to reuse the volume until some capacity is freed.

When a disruption occurs either a volume is considered to be **recoverable** or it is considered **unrecoverable**.

#### Unrecoverable volume

`Unrecoverable` is a state that can be reached in two situations:

* The administrator knows that the volume can't be recovered and it must be abandoned.
* The Elastic operator creates a new pod in an attempt to reuse the volume but the pod is still not scheduled after a given amount of time.

#### Recovering strategies

There are 2 possible strategies when it is time to try to recover the data from a PVC:

##### Recoverable required

The Elastic operator **must not delete** a PVC that may hold the only copy of some data.
`Recoverable required` is a state in which the volume **must** be recovered to get the missing data back online.

##### Recoverable optional

`Recoverable optional` is a state where the missing data is available on some others nodes. For instance if a K8S node with a local volume is down
and if data can be replicated from other nodes then it is not mandatory for the Elastic operator to wait forever.

It is a best effort scenario, we have to choose between:

* Wait for the node to be back online

VS

* Paying the cost of a replication from other nodes

It means that in such scenario we have to find a way to determine the time the operator will wait before it is decided that a new pod must be created.
It may be hard to find this exact timeout, it must be user configurable but a sane default value should be set, based on some criteria like, for example:
* the shard size
* the number of replicas still available

TODO: check if the controller has access to PVC but not to PV or nodes, we can't watch nodes or PV, we can't only watch the claims

## Decision Drivers
The solution must be able to handle the following use cases:

### UC1: The K8S cluster is suffering a external involuntary disruption and the volumes cannot be recovered

In this scenario we must consider the data as permanently lost _(for example vm with local storage has been destroyed)_.

We need to give a way to the user to instruct the Elastic operator that:
* It should immediately move the volume into a `Recovering` strategy.
* If the volume is in the `Recoverable required` state the user should be able to forcibly not reuse the PVC, even if there is no other replica available.

### UC2: The K8S cluster is suffering a external involuntary or voluntary disruption but the volumes can be eventually recovered

The Elastic operator will create a new pod and according to the PV affinity the scheduler will hopefully find a new node where the data is available.
If it takes to much time to schedule the pod then the volume is moved into one of the two `Recoverable` states.

### UC3: As an admin I want to plan a voluntary disruption and the volumes cannot be recovered

In this scenario the administrator want to definitively evacuate a node and the data will not be available
anymore (for example, a server with a local storage is definitively removed from the cluster)

It is usually done in two steps:

1. Cordon the node
1. Evict or delete the pods

## Considered options

### Option 1: Add a finalizer to the PVC

A PVC that is used by a pod will not be deleted immediately because of a finalizer set by the scheduler.
We can add our own finalizer to:
1. Create a new pod
1. Migrate the data and delete the pod.
Once the pod has been deleted the PVC can be deleted by K8S.

### Option 2: handle PVC deletion with an annotation

A tombstone is set on the PVC as an annotation.
The annotation `elasticsearch.k8s.elastic.co/delete` can be set on a PVC with the following values:

* `graceful`:  migrate the data, delete the node and the PVC.
* `force`: discard the data, the operator does not try to reuse the PVC, the PVC is deleted by the Elastic operator.


### Option 3: Add a kubectl plugin to add some domain specific commands

`kubectl` can be extended with new sub-commands: https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/

for example:
```bash
$ kubectl elastic migrate elasticsearch-sample-es-qlvprlqnnk -n default # will migrate the data then delete the pod and the pvc
$ kubectl elastic delete elasticsearch-sample-es-qlvprlqnnk -n default # will delete the pod **and** the pvc
```

### Option 4

Try to handle pod eviction and PVC deletion with a webhook.

## Pros and Cons of the Options

### Option 1

Pros:
* Looks like a simple approach, just do a `kubectl delete pvc/XXXXX` to migrate the data and delete the pod.

Cons:
* If the volume can't be recovered the user will have to remove the `Finalizer`
* Administrator has to `uncordon` the node and delete manually _(also known as error prone)_ the `PVC` if he wants to drain it.

### Option 2
Pros:
* The Elastic operator can figure out easily if it must try to migrate the data or abandon the volume because
it can't be recovered.

Cons:
* Admin has to `uncordon` the node and annotate the `PVC` manually _(still error prone)_ if he wants to drain it.
* Admin must remember the annotations

### Option 3
Pros:
* Provides a meaningful interface

Cons:
* Stable?: Even if plugins were introduced as an alpha feature in the v1.8.0 release it has been reworked in v1.12.0
* End user must install the plugin
* Admins still have to evict nodes manually when the node is drained

### Option 4
Pros:
* Integrate smoothly in the `cordon` + `drain` scenario.

Cons:
* It doesn't seem possible to handle a node eviction _(needs to be confirmed)_.
* Setting a webhook requires some privileges at the cluster level.
* Is it even possible to use a webhooks to safely migrate some data when an eviction occurs?

## Links

* Strimzi: [Deleting Kafka nodes manually](https://strimzi.io/docs/master/#proc-manual-delete-pod-pvc-kafka-deployment-configuration-kafka)
* Kubernetes [isBeingUsed](https://github.com/kubernetes/kubernetes/blob/a3ccea9d8743f2ff82e41b6c2af6dc2c41dc7b10/pkg/controller/volume/pvcprotection/pvc_protection_controller.go#L210)
function: A PVC can be deleted *only* if it is not used by a scheduled pod _(including the `Unknown` state)_
