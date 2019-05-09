# Elastic Cloud on Kubernetes (ECK)

Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch and Kibana on Kubernetes.
This is an alpha version.

Current features:

*  Elasticsearch and Kibana deployments
*  TLS Certificates management
*  Safe Elasticsearch cluster configuration & topology changes
*  Persistent volumes usage
*  [Dynamic local persistent volumes provisioning](https://github.com/elastic/cloud-on-k8s/tree/master/local-volume)
*  Custom node configuration and attributes
*  Secure settings keystore updates

Upcoming features:

*  APM
*  Cross-cluster search & replication
*  Rolling update configuration options
*  Inline configuration changes
*  Elasticsearch version upgrade
*  Improved persistent volumes support
*  Operator namespace management options

Supported versions:

*  Kubernetes: 1.11+
*  Elasticsearch: 6.8+, 7.1+

See the [Quickstart](https://www.elastic.co/guide/en/k8s/current/Quickstart.html) to get started with ECK.

![](docs/img/k8s-operator.gif)
