# 1. Stateful set or custom controller

**Update (2019-07-30):** we decided to refactor the code towards using StatefulSets in order to manage Elasticsearch pods. Mostly in order to get closer to Kubernetes standards, simplify PersistentVolumes management, and stay open to future improvements in the ecosystem. For more details, check [the StatefulSets discussion issue](https://github.com/elastic/cloud-on-k8s/issues/1173).

* Status: ~~accepted~~ rejected, superseded by https://github.com/elastic/cloud-on-k8s/issues/1173
* Deciders: @nkvoll
* Date: ~~~2019-02-12~~ 2019-07-30

## Context and Problem Statement

We manage stateful workloads. StatefulSets were designed in order to manage stateful workloads. Why are we not using StatefulSets like everyone else is doing?


## Decision Drivers <!-- optional -->

### StatefulSets overview

> Manages the deployment and scaling of a set of Pods , and provides guarantees about the ordering and uniqueness of these Pods.

Each pod ends up with a sticky network identifier and an ordinal index, reused on rescheduling, along with its persistent storage. There is a global order in pods of a StatefulSet (pod-1, pod-2, pod-3, and so on) which determines scaling and ordering in rolling operations.


### Elasticsearch topologies

In a given StatefulSet, all pods have the exact same spec.

From our perspective, instances can be configured with several options:

- Elasticsearch version
- node types: master-only, master-data, data-only, master-data-ingest, coordinating-only, ML, APM, and so on
- availability zones: as-1, az-2, and so on
- instance configuration type: hot/warm architectures

In complex production-ready scenarios, we might want to configure a cluster with:

- 3 dedicated masters, in 3 different AZs
- 16 dedicated data nodes, in 3 different AZs, 3 of them with hot indices, 13 with warm indices
- 3 ML nodes
- 5 dedicated ingest nodes

Mapping these to StatefulSets definitions would probably lead to at least 5 different StatefulSets resources for a single cluster.

### Rolling upgrades

A good way to rolling-upgrade an Elasticsearch cluster:

1. maybe snapshot the cluster
2. proceed node by node, but probably start with data nodes first and master nodes last
3. call the Elasticsearch API to exclude allocations on the node to be removed
4. wait for shards of this node to be migrated to other nodes
5. safely remove the node
6. add a new node
7. wait for all shards to be replicated properly
8. maybe restore from snapshot if things went wrong
9. move on to next node

Some of these steps are very specific to Elasticsearch, and cannot be accomplished by the StatefulSet controller itself. It needs some extra control on the way nodes are shutdown. The StatefulSet does have various update strategies: `RollingUpdate`, `RollingUpdate.Partition`. The last one offers a bit more control on performing the rolling upgrade in several stages, leaving us some time and control over API calls we need to perform before replacing a node. Which then become quite close to manually updating pods one-by-one. We are still forced to perform the upgrade following ordinal indices defined by the StatefulSet though (unless we have one StatefulSet per node :)).

For certain scenarios, we might prefer upgrading the cluster in a "grow-and-shrink" fashion: add extra nodes, then remove old ones. This is the case of 1-node clusters for instance. It is not easy to achieve with StatefulSets, and probably requires some manual tweaks with partitioned rolling upgrades.

Overall, we would lose a lot of flexibility in the way we'd like to run rolling upgrades by using StatefulSets.


## Considered Options

* (multiple) stateful sets
* implementing our own custom controller


## Decision Outcome

We decided to implement our own controller that manages pods directly.

### Positive Consequences
* The immediate benefit in working with Pods directly is more control over the the cluster lifecycle. We can handle rolling upgrades, version migrations, cluster growth, volume reuse (or not), and multi-AZ orchestration with much more flexibility.
* Relying on StatefulSets forces us to depend on the StatefulSet controller releases, updates and bug fixes, since it has direct control over the pods themselves. 
* Pods are a core concept of K8s, with well-known behaviours thoroughly tested in the field. Their spec and behaviour is less likely to evolve in a direction that does not suit us.
* We could rely on StatefulSets: it would simplify a part of the code, and complexify another part of the code. Because of complex cluster topologies, we would still need to handle several StatefulSets for a single cluster, which is not much more simpler than handling several pods directly. 

### Negative Consequences
* Things we need to reimplement since we're not using StatefulSet
   - Orchestration: need to manually create and delete pods, by comparing expected pods to actual pods. By using StatefulSets, we would only compare StatefulSets specs.
   - Rolling upgrades: need to be manually handled (which gives us more flexibility).
   - Non-determistic identities and cache inconsistencies: need to handle potential resources cache inconsistencies by relying on Expectations, similar to ReplicaSets.



## Links

- [StatefulSets documentation](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)

- [etcd-operator](https://github.com/coreos/etcd-operator): one of the first and most popular operator out there. Does not rely on StatefulSets. Check [this comment](https://github.com/coreos/etcd-operator/issues/1323#issuecomment-317875165) from _xiang90_:
> Statefulset is not flexible enough to achieve quite a few things easily, and the benefits it bring in right now are not significant.

- [Best practices for building Kubernetes Operators and stateful apps (Google)](https://cloud.google.com/blog/products/containers-kubernetes/best-practices-for-building-kubernetes-operators-and-stateful-apps)
> For example, you can use the StatefulSet workload [...]. However, for many advanced use cases such as backup, restore, and high availability, these core Kubernetes primitives may not be sufficient. That’s where Kubernetes Operators come in.

- [The sad state of stateful Pods in Kubernetes](https://elastisys.com/2018/09/18/sad-state-stateful-pods-kubernetes/)
> The problem is that StatefulSet does not understand anything about what is going on inside the stateful Pods. It is an abstraction layer, and by definition, abstractions are bad at dealing with details.
