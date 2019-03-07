# 6. Elasticsearch sidecar health and cluster data collection

* Status: proposed
* Deciders: k8s-operators team
* Date: 2019-03-05

## Context and Problem Statement

This proposal aims to outline possible approaches to report the Elasticsearch sidecar health in combination with the cluster state for cluster-wide monitoring and alerting.

As for now an Elasticsearch pod is composed of 2 containers: 
- a main container for Elasticsearch
- a sidecar container for running the keystore-updater

What is the keystore-updater in the sidecar doing?
It calls the Elasticsearch endpoint `/_nodes/reload_secure_settings` to decrypt and re-read the entire keystore used by the snapshotter job. 
To connect to ES it depends on:
- an environment variable for the username
- secrets mounted as readonly files for the password and the CA certificate
- the Elasticsearch readiness

Currently there is no health check based on the state of the sidecar. The sidecar can error without anyone ever noticing this state.
So there is a need to check that everything is correctly setup in the sidecar container and the call to the ES API succeeds. 

If the sidecar container is not ready, the Elasticsearch container is impacted because the pod is considered not ready and 
Kubernetes stops to send traffic to the pod. We must accept that the two containers are intimately linked. A sidecar failure
can impact the Elasticsearch availability by design.

However Go binaries that do simple things are very fast to start and very reliable. 
From that we could admit that the probability to have a failure in the sidecar that runs a simple go binary is very low 
comparing to have an Elasticsearch failure.

Another challenge is to take into account that some sidecar errors are to be expected when ES is not ready yet.

This can be mitigated by considering a start-up delay during which it is accepted that ES is not ready and 
do not report errors during this period. Then how to detect that ES has never started?
The ES readiness probe will fail if ES never becomes ready.

To take decisions and schedule actions to repair broken clusters, we want also to collect the cluster states. 
The sidecar seems to be a good place to do that.
Could the sidecar be used to poll the Elasticsearch health and the cluster state?
One of the benefits would be that the controller can only interface with the sidecar.

## Decision Drivers

* Error distinction: a sidecar failure should be easily identified from an Elasticsearch failure
* Error side effect: a sidecar failure should not increase the unvailability of Elasticsearch compared to the current situation
* Promote reliability and simplicity because health-checking is a critical part of the system
* Allow the collection of the cluster state

## Considered Options

* 1: The Kubernetes-way with a liveness probe + expose an HTTP health endpoint
* 2: The Kubernetes-way with a readiness probe + expose an HTTP health endpoint
* 3: Logging-based error reporting

## Decision Outcome

Chosen option: option 1, because it gives us more flexibility, it's pretty simple to implement and it does not depend
on external components.

### Positive Consequences

* Much more flexibility to interact with the pod through the HTTP server in the sidecar
* Minimize external dependencies

### Negative Consequences

* Increase a little the failure domain of the sidecar with the presence of the HTTP server

## Pros and Cons of the Options

### Option 1: The Kubernetes-way with a liveness probe

The keystore-updater sidecar exposes an HTTP endpoint `/live` consumed by a liveness probe.
This endpoint returns a success as long as ES is not ready. Then when ES is ready, it returns a response reflecting the state of
the call to the ES API. ES is considered ready when `/` is reachable.
This solution implies that the probe through the sidecar will poll Elasticsearch.

* Good, because it's well integrated with Kubernetes. Kubernetes will restart the sidecar container if the probe fails. It could
 probably resolve some issues related to the state of the system (e.g. out of memory, too many connections).
* Good, because exposing the health over HTTP allows easily the collect by other systems
* Good, because it's easy to expose the cluster state in another HTTP endpoint
* Bad, because Kubernetes will restart the sidecar container if the liveness probe fails. And if that happens indefinitely, the
sidecar container will reach the CrashLoopBackOff status, then it won't be ready and the ES service will be impacted.

### Option 2: The Kubernetes-way with a readiness probe

Same as option 1 but the endpoint is consumed by a readiness probe. The goal is to ensure that the keystore-updater was able
to call the ES API at least once to prevent errors coming from a bad configuration.

* Good, because it's well integrated with Kubernetes.
* Good, because exposing the health over HTTP allows easily the collect by other systems
* Good, because it's easy to expose the cluster state in another HTTP endpoint
* Bad, because Kubernetes will stop to send traffic to ES if the readiness probe fails.

### Option 3: Logging-based error reporting

This solution consists of limiting the reporting of errors in the logs of the sidecar.
Then retrieve and ship container logs using an agent to an ES cluster dedicated to monitoring (could be Filebeat or Fluent Bit).
The cluster state can also be logged to be collected and then aggregated in Elasticsearch.

* Good, because it's pretty simple implement
* Good, because it's completely isolated from Elasticsearch
* Bad, because it makes the failure detection dependent on a log collection pipeline
* Bad, because it's not the most trivial way to expose the cluster state

## Links

* [Discussion issue](https://github.com/elastic/k8s-operators/issues/432)
* https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/
* https://www.elastic.co/guide/en/beats/filebeat/master/running-on-kubernetes.html
