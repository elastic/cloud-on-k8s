# Beats Configuration Examples

This directory contains yaml manifests with example configurations for Beats. These manifests are self-contained and work out-of-the-box on any non-secured Kubernetes cluster. All of them contain three-node Elasticsearch cluster and single Kibana instance. All Beat configurations set up Kibana dashboards if they are available for a given Beat and all required RBAC resources. 


#### Metricbeat for Kubernetes monitoring - `metricbeat_hosts.yaml`

Deploys Metricbeat as a DaemonSet that monitors the host resource usage (cpu, memory, network, filesystem) and Kubernetes resources (Nodes, Pods, Containers, Volumes).

#### Filebeat with autodiscover - `filebeat_autodiscover.yaml`

Deploys Filebeat as DaemonSet with autodiscover feature enabled. All pods in all namespaces will have logs shipped to an Elasticsearch cluster.

#### Filebeat with autodiscover for metadata - `filebeat_autodiscover_by_metadata.yaml`

Deploys Filebeat as a DaemonSet with autodiscover feature enabled. Fullfilling any of the two conditions below will cause a given Pod's logs to be shipped to an Elasticsearch cluster:

- Pod is in `log-namespace` namespace
- Pod has `log-label: "true"` label 

#### Filebeat without autodiscover - `filebeat_no_autodiscover.yaml`

Deploys Filebeat as a DaemonSet with autodiscover feature disabled. Uses entire logs directory on the host as the input. Doesn't require any RBAC resources as no Kubernetes APIs are used.   

#### Metricbeat for Elasticsearch Stack Monitoring - `stack_monitoring.yaml`

Deploys Metricbeat configured for Elasticsearch and Kibana [`Stack Monitoring`](https://www.elastic.co/guide/en/kibana/current/xpack-monitoring.html). Deploys one monitored Elasticsearch cluster and one monitoring Elasticsearch cluster. You can access the Stack Monitoring app in the monitoring cluster's Kibana. Combine with one of the Filebeat recipes but pointed to the `elasticsearch-monitoring` cluster for the full Stack Monitoring experience.

#### Heartbeat monitoring Elasticsearch and Kibana health - `heartbeat_es_kb_health.yaml`

Deploys Heartbeat as a single Pod deployment that monitors the health of Elasticsearch and Kibana by TCP probing their Service endpoints. Note that Heartbeat expects that Elasticsearch and Kibana are deployed in the `default` namespace.

#### Auditbeat - `auditbeat_hosts.yaml`

Deploys Auditbeat as a DaemonSet that checks file integrity and audits file operations on the host system.

#### Journalbeat - `journalbeat_hosts.yaml`

Deploys Journalbeat as a DaemonSet that ships data from systemd journals.

#### Packetbeat monitoring DNS and HTTP traffic - `packetbeat_dns_http.yaml`

Deploys Packetbeat as a DaemonSet that monitors DNS on port `53` and HTTP(S) traffic on ports `80`, `8000`, `8080` and `9200`.
