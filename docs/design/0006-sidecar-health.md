# 6. Elasticsearch sidecar health

**Update (2019-07-30):**
There is no sidecar anymore. The process-manager has been removed for simplification, and the keystore updater now runs as an init-container. Hence, there is no need to monitor the sidecar health anymore.

* Status: proposed
* Deciders: cloud-on-k8s team
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
compared to have an Elasticsearch failure.

Another challenge is to take into account that some sidecar errors are to be expected when ES is not ready yet.

This can be mitigated by considering a start-up delay during which it is accepted that ES is not ready and 
do not report errors during this period. Then how to detect that ES has never started?
The ES readiness probe will fail if ES never becomes ready.

## Decision Drivers

* Error distinction: a sidecar failure should be easily identified from an Elasticsearch failure
* Error side effect: a sidecar failure should not increase the unavailability of Elasticsearch compared to the current situation
* Promote reliability and simplicity because health-checking is a critical part of the system

## Considered Options

* 1: The Kubernetes-way with a liveness probe + expose an HTTP health endpoint
* 2: The Kubernetes-way with a readiness probe + expose an HTTP health endpoint
* 3: Logging-based error reporting
* 4: Operator polling + events sending + expose an HTTP health endpoint

## Decision Outcome

Chosen option: option 4, because it gives us more flexibility to take decisions in case of failure, it does not depend on Kubernetes probes/kubelet and it does not depend on external components.

### Positive Consequences

* Collecting the sidecar health from the operator side gives us more options to react to failures
* Having an HTTP server in the sidecar brings more flexibility to interact with the pod
* Does not depend on the Kubernetes probes or the Kubelet
* Minimize external dependencies

### Negative Consequences

* Increase a little the failure domain of the sidecar with the presence of the HTTP server
* Add complexity and responsibility to the operator

## Pros and Cons of the Options

### Option 1: The Kubernetes-way with a liveness probe

The keystore-updater sidecar exposes an HTTP endpoint `/live` consumed by a liveness probe.
This endpoint returns a success as long as ES is not ready. Then when ES is ready, it returns a response reflecting the state of
the call to the ES API. ES is considered ready when `/` is reachable.
This solution implies that the probe through the sidecar will poll Elasticsearch.

* Good, because it's well integrated with Kubernetes. Kubernetes will restart the sidecar container if the probe fails. It could
 probably resolve some issues related to the state of the system (for example, out of memory, too many connections).
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

### Option 4: Polling from the operator and sending events

Same as option 1 and 2 in terms of:
- exposing an HTTP endpoint with the health of the sidecar
- considering the sidecar healthy when ES is not ready during its startup
Then, the operator polls this endpoint and reports any change in the health status as an event.

Sending the status as an event has the added benefit of giving us richer behaviour on top of the polling.
It makes the state of the sidecar process observable. We could for example return a revision of the secrets the sidecar process has
seen in the response to the health/status check and implement coordinated behaviour on top of that: for example, do x only if all sidecars have seen secret revision y.

* Good, because it does not use readiness/liveness probes that can provoke a container restart or a service unavailability
* Good, because it can give us an aggregated view of all the sidecar healths
* Good, because it makes the state of the sidecar process observable
* Good, because it gives more options to react to failures
* Bad, because it increases the responsibilities of the operator
* Bad, because it adds more complexity to the operator

## Links

* [Discussion issue](https://github.com/elastic/cloud-on-k8s/issues/432)
* https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/
* https://www.elastic.co/guide/en/beats/filebeat/current/running-on-kubernetes.html
