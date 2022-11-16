# Handling total capacity requirements for local volumes

* Status: proposed
* Deciders: k8s operators team
* Date: 2019-02-25

## Context and Problem Statement

Our current [dynamic provisioner for local volumes](https://github.com/elastic/local-volume) does not handle maximum storage available on nodes. It means a pod can get assigned to a node for which we'd need to create a PersistentVolume, even though the physical disk might be full already.

The way it currently works is the following:

1. a single cluster-wide provisioner dynamically creates PersistentVolume resources matching non-bound PersistentVolumeClaim resources. These PersistentVolumes are not bound to any particular node yet, they will be once the pod is scheduled to a node.
2. a driver on each node is responsible for creating the actual volume on disk when a pod is scheduled to the node. It also updates the PersistentVolume NodeAffinity to match the node (for PV reuse).

In step 1, we do not assign the PV to any node yet. The pod can be scheduled to any node by the kubernetes scheduler. In step 2, the pod might have been assigned to a node with no free space.

We need a way to make sure pod scheduling takes available disk space into consideration.

## Decision Drivers

* A Pod requesting an `elastic-local` PVC which would not fit on a node's remaining disk space should not be scheduled on this node.
* The solution should not require any particular human operator intervention.
* Performance impact in kubernetes pod scheduling should be acceptable.
* Node affinity and failure domain mapping should still be possible.

## Considered Options

### Option 1: consider disk space as a multiplier of RAM capacity

One way to circumvent these problems is to consider that each kubernetes node is composed of:

* X amount of RAM capacity (for ex. 10GB)
* Y amount of disk space (for ex. 1TB)
* A ration Y/X can be expressed as the ram-to-disk storage multiplier (for ex. 100 in this scenario)

If all nodes in the kubernetes cluster respect this approach and use the same ram-to-disk multiplier, we can consider the node will run out of RAM (hence be unschedulable for new pods) before it runs out of disk.

It requires:

* kubernetes cluster administrator (human) to be aware of this multiplier and size kubernetes nodes accordingly
* users of the operator to be aware of this multiplier and size elasticsearch nodes accordingly (either manually map RAM to disk numbers, or provide only RAM and let disk computation to the elastic operator)

### Option 2: one PV for the entire disk space, but bind to smaller ones

#### Node volume provisioner

Instead of having a single cluster-wide provisioner, we have one provisioner per node, responsible for provisioning PersistentVolume corresponding to the node. Let's call it the "node volume provisioner" (name TBD). The global provisioner does not exist anymore.

On startup, the node volume provisioner inspects the available disk space (for ex. 10TB total). It creates a single PersistentVolume resource on the apiserver, with node affinity set to the node it's running on. This PersistentVolume covers the entire available disk space (10TB). This PersistentVolume is not bound to any PersistentVolumeClaim yet. So far, this is quite similar to what's done by the [static local volume provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner), except we probably want to consider a single PV with the entire LVM disk space (spanning over multiple disks) instead of creating one PV per disk.

#### PVC/PV binding

When a PVC with storage class `elastic-local` is created, the Kubernetes PersistentVolume controller will automatically bind the PersistentVolume created by the node volume provisioner to this PVC (or the PVC created by another node). Storage capacity is taken into consideration here: if the PV spec specifies a 10TB capacity, it will not be bound to a PVC claiming 20TB. However, a 1GB PVC can still be bound to our 10TB PV. Effectively wasting our disk space here.

So how do we avoid wasting disk space in this scenario? As soon as the PV is bound to a PVC, our node volume provisioner gets notified (it's watching PVs it created). By retrieving both PV and matching PVC, it notices the PVC requests only 1GB out of the 10TB available. As a result, it updates the PersistentVolume spec to match those 1GB. The PV stays bound to the same PVC, even though its capacity was changed. The actual volume corresponding to this PV can then be created by the driver running on the node, as done in the current implementation.

We are left with 9.999TB available on the node: the node volume provisioner creates a new PersistentVolume with capacity 9.999TB, that can be bound to any PVC by the kubernetes PersistentVolume controller. If any PV gets deleted, the node volume provisioner reclaims the disk space freed by updating the PV capacity. For example, if the 1GB pod is deleted, the 9.999TB PV can be updated to 10TB.

To summarize:

* the node provisioner makes sure there is always one PV at a given time, waiting to be bound to a PVC by kubernetes. This PV covers the entire available disk space. If more disk space is available locally, it is reflected by the PV, controlled by the provisioner.
* when the PV gets bound to a PVC, the node provisioner resizes it to match the PVC capacity, and creates a new PV to cover the remaining disk space.

#### Deployment

The node volume provisioner can be deployed as a DaemonSet on all desired nodes, and need RBAC permissions to:

* get, list PersistentVolumeClaims
* get, list, create, update, delete PersistentVolumes

It can be running either:

1. as a container in a new pod
2. as a container in the same pod as the driver
3. alongside the driver process (same Go binary)

#### Limitation: scheduling concurrency

Due to the fact each node volume provisioner always has a single PV waiting to be bound at a given time (representing total available disk space at that time), kubernetes cannot assign multiple PVCs to the same underlying node at once. Once the first PVC is bound, a second PVC can only be bound after a new PV representing the remaining disk space is created by the provisioner.

In scenarios where we deploy 3 Pods (in a Deployment for instance) with a PVC for each, chances are all 3 Pods will be assigned to different nodes. Whereas they could have been assigned to the same node if PV creation was not managed by the node volume provisioner.

The time necessary to create a new PV covering the remaining free space can be estimated to be rather small though. Once notified about the PV/PVC binding, the provisioner can directly issue a new PV. That's in the order of milliseconds in the best-case scenario.

One way to mitigate this could be to maintain more than one PV waiting to be bound. For example, by splitting the 10TB space to 10 times 1TB space.

#### Failure domains

This approach does not affect failure domain in any way. Failure domains can be expressed through labels on the pod. By relying on the `WaitForFirstConsumer` volume binding mode in the storage class (Kubernetes 1.12), the pod will be scheduled on a node before its PVC gets bound to a PV. Priority is therefore given to the pod failure domain criteria before a PVC/PV mapping is picked.

#### Security considerations

The provisioner pod on each node must have read/write/update/delete access to PersistentVolume resources. This is already the case for the current local-volume driver. This means the driver has the required API access to interfere with other PersistentVolumes in the cluster, from other storage classes. A local container-escaping exploit could technically be used to get access to perform CRUD operations against all PVs in the cluster, and possibly get access to PVs that are non-local.

#### Risks

* If Kubernetes team decides to forbid updates on PersistentVolumes, the current design will not work as expected, since we rely on updating PVs with the proper reduced disk capacity after they are bound. The possibility this could happen seems rather low, considering the recent PVC/PV expansion feature that also requires updating both PVC and PV.

## Decision Outcome

TODO. Option 2 seems way better IMHO.

## Pros and Cons of the Options

### Option 1

Pros:

* No change in the local-volume provisioner design
* No need to bother about "remaining disk space", as long as kubernetes nodes are sized accordingly

Cons:

* PVs on nodes that are left for reuse and are not used by any running pod are not taken into consideration. We might end up in a situation where the node is "full" because all the RAM is used, but there was a PV left alive for reuse that cannot be used anymore (would need some free RAM).
* Not flexible to easily map to a lot of configurations
* Does not apply well outside the context of deploying Elasticsearch clusters
* Does require some knowledge from both the kubernetes administrator and the elastic operator user

### Option 2

Pros:

* Gives flexibility to assign any disk capacity to any pod
* No additional operation required from the user (k8s administrator) point of view
* Each node is responsible for its own PVs. Sounds like a scalable design (as long as requests to the apiserver correctly scale)?

Cons:

* Only one PVC can be mapped to a node at a given time: does impact and/or slow down kubernetes scheduling (only for pods claiming this storage class)
* Updating PVs after they are bound, while possible, sounds a bit like "a hack"

## Links <!-- optional -->

* [Elastic dynamic provisioner for local volumes](https://github.com/elastic/local-volume)
* [Kubernetes static local volume provisioner](https://github.com/kubernetes-sigs/sig-storage-local-static-provisioner)
* [Local volume initial issue](https://github.com/elastic/cloud-on-k8s/issues/108)
