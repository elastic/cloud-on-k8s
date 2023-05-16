= GKE Autopilot Configuration Examples

This directory contains a yaml manifest with an example configuration for running Elasticsearch, Kibana, Fleet Server, Elastic Agent and Metricbeat on GKE Autopilot. These manifests are self-contained and work out-of-the-box on any GKE Autopilot cluster with a version greater than 1.25.

IMPORTANT: These examples are for illustration purposes only and should not be considered to be production-ready.

NOTE: The Elasticsearch example uses a Daemonset to set to ensure that `/proc/sys/vm/max_map_count` is set on all of the underlying Kubernetes nodes for optimal performance. See https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-virtual-memory.html for more information.

==== Elasticsearch, Kibana and Elastic Agent in Fleet mode

===== Agent with System and Kubernetes integrations - `elasticsearch.yaml`+`fleet-kubernetes-integration.yaml`

Deploys Elastic Agent as a DaemonSet in Fleet mode with System and Kubernetes integrations enabled. System integration collects syslog logs, auth logs and system metrics (for CPU, I/O, filesystem, memory, network, process and others). Kubernetes integrations collects API server, Container, Event, Node, Pod, Volume and system metrics.

===== Kubernetes integration - `elasticsearch.yaml`+`kubernetes-integration.yaml`

Deploys Elastic Agent as a DaemonSet in standalone mode with Kubernetes integration enabled. Collects API server, Container, Event, Node, Pod, Volume, System, Volume, and State metrics for containers, daemonsets, jobs, nodes, persistent volumes/claims, pods, replicasets, resourcequotas, services, statefulsets, and storageclasses.

==== Metricbeat for Kubernetes monitoring - `elasticsearch.yaml`+`metricbeat_hosts.yaml`

Deploys Metricbeat as a DaemonSet that monitors the host resource usage (CPU, memory, network, filesystem) and Kubernetes resources (Nodes, Pods, Containers, Volumes).