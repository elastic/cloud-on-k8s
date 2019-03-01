# PVC reuse behavior spec

The aim of this document is to capture some scenarios where a pvc get orphaned and define how the “reuse pvc” mechanism must behave.
As a preambule before we dive into the different use-cases and scenarios here are some considerations about what can lead to a disruption and a reminder about some constraints raised by storage classes.

## About disruptions

A Pod does not disappear until a person or a controller delete it or there is an unavoidable hardware or system software error.
The reason why a PVCs can be abandoned by a Pod can be classified into 3 main categories :

* There is an **external involuntary** disruption :
  * Hardware failure
  * VM instance is deleted
  * Kernel panic
  * Eviction of a pod due to the node being out-of-resources
* There is an **external voluntary** disruption
  * The node hosting the Pod is drained because :
    * The K8S node is about to be upgraded or repaired
    * The K8S cluster is scaling down
  * The pod is manually deleted by someone (not only as an error but also because sometime a reboot can fix a problem)
* There is a **voluntary disruption driven by the Elastic operator**, the pod is deleted by the reconciliation loop.
  * The volume can’t be reused if :
    * The user want a major topology change (e.g. moving a ES node from a availability zone to another one)
    * The pod deletion is part of a “grow-and-shrink” process.
    * User requires more capacity (e.g. CPUs) than the node can offer
  * The volume can be reused if the spec of the cluster has been changed with something that is “compatible” with an “inline” upgrade and if resources are available on a node where the volume can be attached. It can be seen as something that could be done by a human operator on a bare metal infrastructure, for instance :
    * Capacity changes (e.g. add some memory)
    * Elasticsearch update, minor and major

## Storage class constraints

Storage classes do not all provide the same capabilities when it comes to reuse a volume, for instance :

* Google persistent disks can be attached from a single availability zone
* Regional persistent disks replicate the data between 2 zones in the same region
* A volume backed by our elastic-local storage class can only be reused on the same node

At this stage it is worth mentioning that even if the K8S scheduler uses some predicates to reschedule a pod on a node where the volume can be reused or attached, it does not preserve the capacity needed to reschedule the pod. For instance if a pod was using a local volume and if the node runs out of capacity while the pod is being recreated then it becomes merely impossible to reuse the volume until some capacity is freed.

When a disruption occurs either a volume is considered to be **recoverable** or it is considered **unrecoverable**. It does not depend on the storage class, it might take a longer time to recover for a local storage *(maybe you have to repair the server)*, while for shared storage we *just* need to find a server that can attach the volume. In the same way a volume might be considered unrecoverable for any storage class even it is unlikely to occur for a shared storage.

## Scenarios

### UC1 : The K8S cluster is suffering a external involuntary disruption and the volumes cannot be recovered

In this scenario we must consider the data as permanently lost _(e.g. vm with local storage has been destroyed)_. It can't be detected automatically, so we need to give a way to the user to instruct the Elastic operator that :

* It can get rid of the pvc.
* It must create a new pod.

### UC2 : The K8S cluster is suffering a external involuntary or voluntary disruption but the volumes can be eventually recovered

This is a simple scenario, the Elastic operator will create a new pod and according to the PV affinity the scheduler will eventually find a new node where the data are available.

### UC3 : As an admin I want to plan a voluntary disruption and the volumes cannot be recovered

In this scenario the administrator want to definitively evacuate a node and it is known that the data will be lost (e.g. : a server with a local storage is definitively removed from the cluster)

It is usually done in two steps :

1. Cordon the node
1. Evict or delete the pods

We have several options to tackle this situation :

__Option 1__ : The node is drained, the volume is lost and the user must use the same solution that for UC1

__Option 2__ : We want to offer a clean way to remove the node from the Elasticsearch cluster

### UC4 : As an admin I want to apply a change to an Elasticsearch cluster that is compatible with a “inline” upgrade

The spec of the pods have changed but we can use the same volume, also we can tolerate an undersized cluster and H.A. is not impacted. This compatiblity should be detected by the operator.

The operator could act that way :

1. The pod is deleted but the PVC is preserved.
2. A new pod is scheduled and it reuses the PVC

Maybe that for some cases we can do a kind of "sanity check" before the pod is deleted (e.g. local-storage + capacity changes : does the node has enough capacity ?)

### UC5 : As an admin I want to apply a change to an Elasticsearch cluster that is not compatible with an “inline” strategy or even if it is compatible with a “inline” upgrade I would rather choose a “grow-and-shrink” strategy

IIRC this is what is already implemented, the pvc should be deleted as soon as the pod is deleted, so may be that this scenario is not a use case.

## Considered Options for UC1 and UC3

In UC1 and UC3 a volume can't be reuse or the cluster admin want to drain a node.

### Option 1 : handle PVC deletion with an annotation

A tombstone is set on the PVC as an annotation. The annotation `elasticsearch.k8s.elastic.co/delete` can have two values :

* `graceful` :  migrate the data, delete the node and the PVC.
* `force` : discard the data, the operator does not try to reuse the PVC, the PVC is deleted by the Elastic operator.

Pros :

* Could be a first easy way to get rid of a volume or safely migrate some data

Cons :

* Admin must remember the annotations

### Option 2 : Add a kubectl plugin to add some domain specific commands

`kubectl`  can be extended with new sub-commands : https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/

e.g. :
```bash
$ kubectl elastic migrate elasticsearch-sample-es-qlvprlqnnk -n default
```

Pros :

* Provides a meaningful interface

Cons :

* Stable ? : Even if plugins were introduced as an alpha feature in the v1.8.0 release it has been reworked in v1.12.0
* Admins stil have to evict nodes manually when the node is drained

### Option 3 : handle pod eviction and PVC deletion with a webhook

TODO : is it possible to use mutating webhooks to safely migrate some data when an eviction occurs ?