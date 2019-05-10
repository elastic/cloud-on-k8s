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

Supported versions:

*  Kubernetes: 1.11+
*  Elasticsearch: 6.8+, 7.1+

From here, you can either use the [Quickstart](https://www.elastic.co/guide/en/k8s/current/quickstart.html) to deploy you first cluster with ECK, or you can discover more about the following projects:

*   [Elastic operators and controllers for Kubernetes](https://github.com/elastic/cloud-on-k8s/blob/master/operators/README.md)
*   [Dynamic provisioner for local volumes](https://github.com/elastic/cloud-on-k8s/blob/master/local-volume/README.md)

![](docs/img/k8s-operator.gif)
