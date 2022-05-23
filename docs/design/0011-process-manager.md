# 11. Process manager for keystore update and Elasticsearch restart

* Status: rejected (implementation removed) in July 2019. The keystore updater is moved into an init container to adopt the same 
pattern used for Kibana and APM server. With the move towards the StatefulSet way of doing rolling restarts, we no longer need a
process manager to perform Elasticsearch cluster restart. The zombie reaping problem only exists when another process is started
in the container (for example kubectl exec) and should probably be handled in the official images.
Main benefits are code and architecture simplicity as well as respecting k8s standards.
* Deciders: cloud-on-k8s team
* Date: 2019-05-28

## Context and Problem Statement

This design proposal outlines why a process manager is injected into the standard Elasticsearch container.
The existence of the process manager is driven by three needs: 
* be able to update the Elasticsearch keystore 
* perform full Elasticsearch cluster restart
* solve the PID 1 zombie reaping problem

### Elasticsearch keystore

We chose to provide a Kubernetes native way to users to update the Elasticsearch keystore through a Kubernetes Secret.

The keystore updater is a custom go binary that watches a Volume mounted from a Kubernetes Secret and synchronizes its content in the Elasticsearch keystore. Users just have to update the Secret to update the Elasticsearch keystore.

The keystore updater must be able to access the elasticsearch-keystore binary, request the Elasticsearch API endpoint 
and be run in a long-running process.

The problem is knowing where to run the keystore updater.

### Full Elasticsearch cluster restart

We established that we need to be able to schedule full cluster restart to optimize cluster mutation by reusing pods. 
This is detailed in this [design proposal](https://github.com/elastic/cloud-on-k8s/blob/main/docs/design/0009-pod-reuse-es-restart.md).

The primary use case for pods reuse was switching for one license type to another with network configuration change. 
Non-TLS to TLS migration is no more relevant as TLS is now in basic.

Reusing pods may be still relevant, where simply restarting Elasticsearch process can be way faster
and more efficient than replacing the pod, especially when there's configuration changes 
(minor configuration changes, plugin changes).

### PID 1 zombie reaping

We want to be sure that the signals are properly handled and the Elasticsearch child processes are correctly reaped 
when the Elasticsearch process terminates.

## Decision Drivers

### Save CPU and memory resources

Having a keystore sidecar container implies allocating more cpu and memory compared to running the keystore-updater process in the
Elasticsearch container.

### Init process

Controlling the init process has been identified as interesting for these benefits:
- handles the PID 1 zombie reaping problem
- gives better control on what defines "healthy" and "ready" for the ES process without exposing ES urls to k8s for that
- makes it easier to perform "nominal restarts" of the ES process without having k8s notice about it

### Full Elasticsearch cluster restart to reuse pods

It must be possible to perform a "full cluster restart" and reuse existing pods.

## Considered Options

Where to run the keystore updater?
* run it in a sidecar container using the standard Elasticsearch image
    * ++ easy to serve an API to report the keystore updater status
    * -- request more cpu and memory to the k8s cluster compared to running the keystore updater in the Elasticsearch container
* run it inside a process manager injected into the standard Elasticsearch container
    * ++ easy to serve an API to report the keystore updater status
    * ~ share cpu and memories with the ES process
    * ~ we have to be extremely conservative with resource usage
* run the process manager in a sidecar with [process namespace sharing](https://kubernetes.io/docs/tasks/configure-pod-container/share-process-namespace/#understanding-process-namespace-sharing)
    * ++ easy to serve an API
    * -- request more cpu and memory to the k8s cluster compared to running the keystore updater in the Elasticsearch container
* run it using an off the shelf process manager into the standard Elasticsearch container
    * -- more processes to run an API, the keystore updater and the ES process

How to perform cluster restart?
* Destroy the pod and recreate it
    * -- depending on storage class we might not be able to recreate the pod where the volume resides. Only recovery at this point is manual restore from snapshot. 
    Considering volumes local to a node: during the interval between the pod being deleted and a new pod being scheduled to reuse the same volume, there is no guarantee
    that no other pod will be scheduled on that node, taking up all resources available on the node, preventing the replacing pod to be scheduled.
* Inject a process manager into the standard Elasticsearch container/image
    * ++ would allow us to restart without recreating the pod, unless we need to change pod resources or environment variables, in which case you have to destroy the pod and recreate it
    * ~ has the disadvantage of being fairly intrusive and complex (copying binaries through initcontainers, overriding command etc)
* Use a liveness probe to make Kubernetes restart the container
    * -- hard to coordinate across an ES cluster
    * -- scheduling of restart is somewhat delayed (maybe not a big issue)
    * -- PID 1 zombie reaping problem
* Recreate cluster and restore from snapshot
    * -- slow
    * -- needs a snapshot repository
* Docker exec
    * -- either user or automated
    * -- PID 1 zombie reaping problem

PID 1 zombie reaping problem?
* Inject a process manager to properly handle signals and reap orphaned zombie processes
* Use [pid namespace sharing](https://kubernetes.io/docs/tasks/configure-pod-container/share-process-namespace/#understanding-process-namespace-sharing)
    * -- it is off by default in the versions of Kubernetes we support (controlled through `--docker-disable-shared-pid=false`) and there is an experimental new way of doing it in 1.14+ which again we don't know if people are using it

## Decision Outcome

To cover these three problems we decided to inject our own process manager into the standard Elasticsearch container 
to run the keystore updater and manage the Elasticsearch process in one container.

### Process manager overview

- Custom binary injected into the standard Elasticsearch container using an init container
- Run the keystore updater in a goroutine inside the process manager process
- Expose HTTPS API to report the keystore updater and the ES statuses
- Expose HTTPS API to perform start, stop and kill of the ES process
- Secure HTTPS API using a dedicated CA
- Handle the ES process termination to recreate the container to avoid any issue with memory non released
- Do not restart the ES process after container recreated only if the stop is scheduled using the /stop endpoint
- Use a file to save the current status of the ES process between restart
- Could replace the init container that serves CSR and ease certificates rotation 

### Positive Consequences

* No extra container that allocates CPU and memory
* Full control to perform cluster restart
* API to expose the keystore updater status and control the ES process

### Negative Consequences

* Share CPU and memory resources with the Elasticsearch container
* A bit intrusive (copying binaries through init containers, overriding command, and so on)
* A bit complex (os signal handling)

## Links

* [https://github.com/elastic/cloud-on-k8s/issues/485] Lightweight process manager issue
* [https://github.com/elastic/cloud-on-k8s/issues/454] Full cluster restart issue 