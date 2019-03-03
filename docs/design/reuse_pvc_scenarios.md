# Volume Management

The aim of this document is to capture some scenarios where a pvc get orphaned and define how the “reuse pvc” mechanism must behave.
As a preamble before we dive into the different use-cases and scenarios here are some considerations about what can lead to a disruption and a reminder about some constraints raised by storage classes.

## About disruptions

A Pod does not disappear until a person or the controller delete it or there is an unavoidable hardware or system software error.
The reasons why a Pod can be deleted can be classified into 3 main categories :

* There is an **external involuntary** disruption :
  * Hardware failure
  * VM instance is deleted
  * Kernel panic
  * Any runtime panic (e.g. containerd crash)
  * Eviction is not supposed to happen because we use QoS
* There is an **external voluntary** disruption
  * The node hosting the Pod is drained because, some _(non exhaustive)_ examples are :
    * The K8S node is about to be upgraded or repaired
    * The K8S cluster is scaling down
  * The pod is manually deleted by someone (not only as an error but also because sometime a reboot can fix a problem)

## Storage class constraints

Storage classes do not all provide the same capabilities when it comes to reuse a volume, for instance :

* Google persistent disks can be attached from a single availability zone
* Regional persistent disks replicate the data between 2 zones in the same region
* A volume backed by our elastic-local storage class can only be reused on the same node

At this stage it is worth mentioning that even if the K8S scheduler uses some predicates to reschedule a pod on a node where the volume can be reused or attached, it does not preserve the capacity needed to reschedule the pod. For instance if a pod was using a local volume and if the node runs out of capacity while the pod is being recreated then it becomes merely impossible to reuse the volume until some capacity is freed.

When a disruption occurs either a volume is considered to be **recoverable** or it is considered **unrecoverable**. `Unrecoverable`  is a terminal state. On the opposite `recoverable` is a state that can be split into 2 subcategories :

### Recoverable optional

It is a state where the data are available on some others nodes. For instance if a KS8 node with a local volume is down and if data can be replicated from other nodes then it is not mandatory for the Elastic operator to wait forever.

It is a best effort scenario, we have to choose between :

* Wait for the node to be back online

VS

* Paying the cost of a replication from other nodes

It means that in such a scenario we have to find a way to determine the time the operator will wait before it is decided that a new pod must be created.
It may be hard to find this exact timeout, it must be user configurable but a sane default value should be set, based on some criteria like, for example, the shard size.

TODO: check if the controller has access to PVC but not to PV or nodes, we can't watch nodes or PV, we can't only watch the claims

### Recoverable required

The Elastic operator must not delete a PVC that may hold the only copy of some data. The volume **must** be recovered to get the missing data back online.

## Scenarios

### UC1 : The K8S cluster is suffering a external involuntary disruption and the volumes cannot be recovered

In this scenario we must consider the data as permanently lost _(e.g. vm with local storage has been destroyed)_. It can't be detected automatically, so we need to give a way to the user to instruct the Elastic operator that :

* It can get rid of the pvc.
* It must create a new pod.

### UC2 : The K8S cluster is suffering a external involuntary or voluntary disruption but the volumes can be eventually recovered

TODO : document what happen if there is a partition or a abrupt shutdown ?

This is a simple scenario, the Elastic operator will create a new pod and according to the PV affinity the scheduler will eventually find a new node where the data are available.

### UC3 : As an admin I want to plan a voluntary disruption and the volumes cannot be recovered

In this scenario the administrator want to definitively evacuate a node and it is known that the data will be lost (e.g. : a server with a local storage is definitively removed from the cluster)

It is usually done in two steps :

1. Cordon the node
1. Evict or delete the pods

We have several options to tackle this situation :

__Option 1__ : The node is marked as unschedulable with `kubectl uncordon my-node`, the admin is expected to delete all the PVC before the node is drained.

__Option 2__ : The node is drained, the volume is lost and the user must use the same solution that for UC1

__Option 3__ : We want to offer a clean way to remove the node from the Elasticsearch cluster <<-- better !!!

### Option 1 : Add a finalizer to the PVC

A PVC that is used by a pod will not be deleted immediately because of a finalizer set by the scheduler.
We can add our own finalizer to create a new pod, migrate the data and delete the pod.
Once the pod has been deleted the PVC can be deleted by K8S.

### Option 2 : handle PVC deletion with an annotation

A tombstone is set on the PVC as an annotation. The annotation `elasticsearch.k8s.elastic.co/delete` can be set.


Pros :

* Could be a first easy way to get rid of a volume or safely migrate some data

Cons :

* Admin must remember the annotations

### Option 2 : Add a kubectl plugin to add some domain specific commands

`kubectl` can be extended with new sub-commands : https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/

e.g. :
```bash
$ kubectl elastic migrate elasticsearch-sample-es-qlvprlqnnk -n default
```

Pros :

* Provides a meaningful interface

Cons :

* Stable ? : Even if plugins were introduced as an alpha feature in the v1.8.0 release it has been reworked in v1.12.0
* Admins still have to evict nodes manually when the node is drained

### Option 4 : handle pod eviction and PVC deletion with a webhook

Pros :

* Integrate smoothly in the `cordon` + `drain` scenario.

Cons:

* It doesn't seem possible to handle a node eviction.
* Setting a webhook requires some privileges at the cluster level.
* Is it even possible to use a webhooks to safely migrate some data when an eviction occurs ?