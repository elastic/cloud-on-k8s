# 6. Elasticsearch sidecar health and cluster data collection

* Status: proposed
* Deciders: k8s-operators team
* Date: 2019-03-05

## Context and Problem Statement

This proposal aims to outline possible approaches to report the Elasticsearch sidecar health in combination with the cluster state for cluster-wide monitoring and alerting.

As for now an Elasticsearch pod is composed of 2 containers: 
- a main container for Elasticsearch
- a sidecar container for running the keystore-updater

What is doing the keystore-updater? 
It calls the Elastisearch endpoint `/_nodes/reload_secure_settings` to decrypt and re-read the entire keystore used by the snapshotter job. 
To connect to ES it depends on:
- an environment variable for the username
- secrets mounted as readonly files for the password and the CA certificate
- the Elastisearch readiness

Currently there is no health check based on the state of the sidecar. So the sidecar can error without anyone ever noticing this state.
Another challenge is to take into account the fact that some sidecar errors are to be expected when ES is not ready yet.

To take decisions and schedule actions to repair broken clusters, we want also to collect the cluster states. 
The sidecar seems to be a good place to do that.
Could the sidecar be used to poll the Elasticsearch health and the cluster state?
One of the benefits would be that the controller can only interface with the sidecar.

## Decision Drivers

* Error side effect: a sidecar failure must not impact the Elasticsearch availability
* Error distinction: a sidecar failure should be easily identified from an Elasticsearch failure
* Promote reliability and simplicity because health-checking is a critical part of the system
* Allow the collection of cluster states

## Considered Options

* The Kubernetes-way with a liveness probe + expose an HTTP health endpoint
* Custom agent deployed in a daemonset + expose an HTTP health endpoint
* Logging-based error reporting
* Metrics-based error reporting

Whatever solution is chosen, it could be good to have a start-up delay during which it is accepted that Elasticsearch is not ready and do not report errors during this period. Then how to detect that ES has never started? The ES readiness probe will here to return an error.

## Decision Outcome

TBD.
Option 1 is a bit risky than other options because non isolated from ES.
Option 3 and 4 are quite similar even if option 3 is a bit more appropriate.

## Pros and Cons of the Options

### Option 1: The Kubernetes-way with a liveness probe

The keystore-updater sidecar exposes an HTTP health endpoint consumed by a liveness probe.
The health check should be something very simple like a ping.

* Good, because it's well integrated with Kubernetes
* Good, because exposing the health over HTTP allows the collect by other systems
* Good, because it's easy to expose the cluster state in another HTTP endpoint 
* Bad, because the sidecar could impact the Elasticsearch availability

Some downsides of this approach could be the inability to distinguish between sidecar failure and Elastisearch failure (e.g. the sidecar down means no ES health data even if ES would be fine). 

However Go binaries that do simple things are very fast to start and very reliable. 
From that we could admit that the probability to have a failure in a sidecar that runs a simple go binary is very low comparing to have an Elasticsearch failure (but this is subject to debate).

### Option 2: Custom agent deployed in a daemonset + expose an HTTP health endpoint

The keystore-updater sidecar exposes an HTTP health endpoint consumed by a custom agent deployed on each node using a daemon set.

* Good, because exposing the health over HTTP allows the collect by other systems
* Good, because it's easy to expose the cluster state in another HTTP endpoint
* Good, because it's completely isolated from Elasticsearch
* Bad, because it involves an additional agent

### Option 3: Logging-based error reporting

This solution consists of limiting the reporting of errors in the logs of the sidecar.
Then retrieve and ship container logs using filebeat to an ES cluster dedicated to monitoring.
The cluster state can also be logged to be collected.

* Good, because it's simple implement
* Good, because it's completely isolated from Elasticsearch
* Bad, because it makes the failure detection dependent on a log collection pipeline

### Option 4: Metrics-based error reporting

Almost the same as the previous option but using metricbeat instead of filebeat.

* Good, because it's relatively simple implement
* Good, because it's completely isolated from Elasticsearch
* Bad, because it makes the failure detection dependent on a metric collection pipeline
* Bad, because it's inappropriate to report cluster status

## Links

* [Discussion issue](https://github.com/elastic/k8s-operators/issues/432)
* https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-probes/
* https://www.elastic.co/guide/en/beats/filebeat/master/running-on-kubernetes.html
