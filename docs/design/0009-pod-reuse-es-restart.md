# Reusing pods by restarting the ES process with a new configuration

* Status: rejected (implementation removed) in July 2019, in favor of moving towards the StatefulSet way of doing rolling restarts. The process-manager is also out of the picture. We don't reuse pods. Main benefits are code and architecture simplicity as well as respecting k8s standards.
* Deciders: cloud-on-k8s team
* Date: 2019-03-20

## Context and Problem Statement

This design proposal is focused on how best to handle Elasticsearch process restart inside a pod, to catch up with configuration changes.

The primary use case for this is switching for one license type to another with network configuration change. For example, switching from a cluster with a Basic license (no TLS) to a cluster with a Gold license (TLS) requires a full cluster restart. It's not possible to "just" replace the pods with new ones here: we would need data to be migrated between pods that cannot communicate with each others. Reusing PersistentVolumes could be a solution here, but may not play well with local persistent volumes bound to a particular node: what if the node gets scheduled to another pod in-between the migration?

Reusing pods may also be useful in other situations, where simply restarting Elasticsearch process can be way faster and more efficient than replacing the pod:

- minor configuration changes
- plugin changes

## Decision Drivers

* It must be possible to perform a "full cluster restart" and reuse existing pods.
* Configuration changes should be propagated through a volume mount, not through the pod spec.
* Operator client cache inconsistencies must be taken into consideration (for ex. don't restart a pod 3 times because of a stale cache).
* Volume propagation time must be taken into consideration (it can take more than 1 minute, the ES restart must not happen before the new configuration is there).
* It should be resilient to operator restart.

## Considered Options

There's a single option outlined in this proposal. [This issue](https://github.com/elastic/cloud-on-k8s/issues/454) contains other draft algorithm implementations.

Other options that are considered not good enough:

* don't handle full cluster restart and license change at all (reject unsupported license migration)
* snapshot the cluster, create a new one, restore from snapshot: requires a snapshot repository to perform a license upgrade
* stop the cluster, create a new one by reusing the first one's persistent volumes: does not play well with local persistent volumes, the new pods may not be schedulable to the same nodes if already full

### Overview

* During pods comparison (expected vs. actual), we are able to decide if a pod can be "reused". Which effectively means changing its mounted configuration and restarting the ES process.
* The actual ES process restart is performed by a process-manager running in the Elasticsearch pod. It listens to an HTTP API for authenticated requests from the operator.
* A restart is divided into 2 steps: stop, then start. They map to 2 different API calls.
* The operator controls pods restarts through a state machine, persisted through annotations in the pod. Within a given "state", all operations are idempotent.
* Configuration is passed through a secret volume mount to the pod. To avoid restarting ES with an out-of-date configuration, the operator requests the process manager for the checksum of the current configuration mounted inside the pod.
* Restarting ES processes in a cluster is orchestrated by the operator. ES restarts can be coordinated (full cluster restart), or performed in a rolling fashion.

### Configuration through a secret volume

We need to move away from storing Elasticsearch settings (from `elasticsearch.yml`) in the pod environment variables, because it prevents us from reusing pods. Instead, we can store the `elasticsearch.yml` file in a secret volume, mounted into the Elasticsearch pod.
Why a secret volume and not a configmap volume? Because it may contain some secrets (for ex. Slack or email credentials).

#### Order of creations

The order in which we create pod and configuration volume is important here.

#### Approach 1: creating the secret first

1. Create the secret
1. Create the pod, using the secret volume

Benefits:

* The volume is already there for the pod to use: pod can run immediately.
* If the operator restarts in-between these 2 steps, the pod wasn't created yet: next reconciliation will just create a new one.

Concerns:

* Volume ownership cannot be set to the pod, since the pod does not exist yet.
* Because of that, we need to garbage-collect secrets that are not associated to any pods.

#### Approach 2: creating the pod first

1. Create the pod, using a secret volume which does not exist yet
1. Create the secret

Benefits:

* Secret's owner reference can be set to the pod: it will be automatically garbage-collected.

Concerns:

* The pod cannot immediately start, and will enter an error state until it the volume is available.
* This can significantly delay pod startup time.
* The operator could restart in-between these two steps: secret configuration reconciliation must be done at every reconciliation, and not only at pod creation time.

#### Chosen approach

The first one (create the secret first): the impact on startup time is important.

### Detecting configuration changes eligible for pod reuse

Some configuration settings are compatible with pod reuse (license type, plugins, minor settings tweaks).
Some aren't (increasing the amount of RAM for the pod).

This can be easily plugged into our existing `comparisons` mechanism. It should report, at the end of a comparison, whether expected and actual match, but also, in case of mismatch, if we can reuse the existing pod to propagate configuration changes.

Changes in licenses are special because they may provoke a full cluster restart. A full cluster restart is required when:

* moving from basic to (trial | gold | platinum) - enable XPack security and TLS
* moving from (trial | gold | platinum) to basic - disable XPack security and TLS

### Reconciliation loop algorithm

Note: this does not represent the _entire_ reconciliation loop, it focuses on the pieces we're interested in.

* Get the ES cluster spec
* Compute expected pods spec
* Compare expected vs. actual
    * Compare the pods spec
    * Compare actual config secret content vs. expected config content
        * If actual config does not exist yet, requeue (stale cache).
    * The comparison result will return
        * Pods to create
        * Pods to delete
* Attempt to find a match for reuse between pods to create and pods to delete
    * This needs to be deterministic among multiple iterations: executing the algorithm twice should lead to the same matches.
    * We now have: pods to create, pods to delete, and pods to reuse.
* Maybe create new pods
    * Check pods expectations
    * Create the configuration secret
    * Create the pod
* Maybe delete deprecated pods
    * Exclude pod from shard allocations
    * Check if the pod does not hold any primary shard, or re-queue
    * Delete the pod
    * Delete the configuration secret (would be garbage collected, but still)
* Handle pods reuse and pods restarts (always executed)
    * Annotate each pod to reuse, unless already annotated
        * `restart-phase: schedule` for restarting the ES process. The operator has no reason to apply this (a human could).
        * `restart-phase: schedule-rolling` for restarting ES processes one by one. The operator would apply this to perform a safe restart (default case).
        * `restart-phase: schedule-coordinated` for all ES processes at once (full cluster restart). The operator would apply this to switch TLS setting following a license change (for ex. basic with no TLS to Gold with TLS).
     * Handle pods in a _schedule_ phase
        * If annotated with `schedule`: annotate them with `restart-phase: stop`.
        * If annotated with `schedule-rolling`
            * If there are some pods in another phase (for ex. `stop`), re-queue.
            * Else, deterministically pick the best pod to get started with, and annotate it with `restart-phase: stop`.
        * If annotated with `schedule-coordinated`: annotate them with `restart-phase: stop-coordinated`.
     * Handle pods in a _stop_ phase
        * If annotated with `stop`
            * Disable shards allocations in the cluster (avoids shards of the temporarily stopped node to be moved around)
            * Perform a sync flush, for fast recovery during restarts
            * Request a stop to the process manager: `POST /es/stop` (idempotent)
            * Check if ES is stopped (`GET /es/status`), or re-queue
            * Annotate pod with `restart-phase: start`
        * If annotated with `stop-coordinated`
            * Apply the same steps as for the `stop` annotation, but:
                * wait for all ES processes to be stopped instead of only the current pod one
                * annotate pod with `restart-phase: start-coordinated`
     * Handle pods in a `start` phase
        * If annotated with `start`:
            * If the pod is marked for reuse, update the configuration secret with the new expected configuration (else, we'll use the current one)
            * Check if the config (expected one if reuse, else current one) is propagated to the pod by requesting the process manager (`GET /es/status` should return the configuration file checksum), or re-queue.
            * Update the pods labels if required (for ex. new node types)
            * Start the ES process: `POST /es/start` (idempotent)
            * Wait until the ES process is started (`GET /es/status`)
            * Enable shards allocations
            * Remove the `restart-phase` annotation from the pod
        * If annotated with `start-coordinated`:
            * Perform the same steps as for the `start` annotation, but wait until *all* ES processes are started before enabling shards allocations
* Garbage-collect useless resources
    * Configuration secrets that do not match any pod and existed for more than for ex. 15min are safe to delete

#### State machine specifics

In the reconciliation loop algorithm, it is important to note that:

* Any step in a given phase is idempotent. For instance, it should be OK to run steps of the `stop` phase over and over again.
* Transition to the next step is resilient to stale cache. If a pod is annotated with the `start` phase, it should be OK to perform all steps of the `stop` phase again (no-op). However the cache cannot go back in time: once we reach the `start` phase we must not perform the `stop` phase at the next iteration. Our apiserver and cache implementation consistency model guarantee this behaviour.
* the operator can restart at any point: on restart it should get back to the current phase.
* a pod that should be reused will be reflected in the results of the comparison algorithm. However, once its configuration has been updated (but before it is actually restarted), it might not be reflected anymore. The comparison would then be based on the "new" configuration (not yet applied to the ES process), and the pod would require no change. That's OK: the ES process will still eventually be restarted with this correct new configuration, since annotated in the `start` phase.
* if a pod is no longer requested for reuse (for ex. user changed their mind and reverted ES spec to the previous version) but is in the middle of a restart process, it will still go through that restart process. Depending on when the user reverted back the ES spec, compared to the pod current phase in the state machine:
    * if the new config was not yet applied, the ES process will still be restarted with its current config
    * if the new config was already applied and the ES process is starting, we'll have to wait for the restart process to be over before the pod can be reused with the old configuration (and restarted again). Depending on the pods reuse choices, we might end up reverting the original configuration to different pods. But eventually things will get back to the expected state.

#### Naming

* Restart annotation name: `elasticsearch.k8s.elastic.co/restart-phase`
* Restart phases:
    * `schedule`, `schedule-rolling`, `schedule-coordinated` represents work and preparation to be done by the operator
    * `stop`, `stop-coordinated` when the ES processes are in the process of being stopped
    * `start`, `start-coordinated` when ES processes are in the process of being started
    * nothing when there is nothing to be done (or ES process restart is over)
 
#### Extensions to other use cases (TBD if worth implementing)

* We **don't** need rolling restarts in the context of TLS and license switch, but it seems easy to implement in the reconciliation loop algorithm, to cover other use cases.
* We could catch any `restart-phase: schedule-rolling` set by the user on the Elasticsearch resource, and apply it to all pods of this cluster. This would allow the user to request a cluster restart himself. The user can also apply the annotation to the pods directly: this is the operator "restart API".
* Applying the same annotations mechanism with something such as `stop: true` could allow us (or the user) to stop a particular node that misbehaves.
* Out of scope: it should be possible to adapt the reconciliation loop algorithm to replace a "pod reuse" by a "persistent volume reuse". Pods eligible for reuse at the end of the comparison are also eligible for persistent volume reuse. In such case, we'd need to stop the entire pod instead of stopping the ES process. The new pod would be created at the next reconciliation iteration, with a new config, but would reuse one of the available persistent volumes out there. The choice between pod reuse or PV reuse could be specified in the ES resource spec?

## Decision Outcome

Chosen option: option 1, because that's the only one we have here? :)

### Positive Consequences

* Handles pod and cluster restart, rolling or coordinated
* Allows humans to trigger a restart through annotations
* Safe from cache inconsistencies, operator restart, reconciliation retries, volume propagation

### Negative Consequences

* Needs to be implemented!
* Additional complexity.
* Need to be extra careful about chaining steps in the right order, and make them idempotent.
* Once a restart is scheduled, it will go through completion. If the user modifies settings again, we'll wait for the current restart to be done.

## Links

* [https://github.com/elastic/cloud-on-k8s/issues/454] Full cluster restart issue
* [https://github.com/elastic/cloud-on-k8s/issues/453] Basic license support issue
* [https://www.elastic.co/guide/en/elasticsearch/reference/7.17/restart-upgrade.html] Elasticsearch full cluster restart upgrade
* [https://www.elastic.co/guide/en/elasticsearch/reference/7.17/rolling-upgrades.html] Elasticsearch rolling cluster restart upgrade
