
[![Build Status](https://devops-ci.elastic.co/buildStatus/icon?job=cloud-on-k8s-e2e-tests&subject=E2E%20tests)](https://devops-ci.elastic.co/view/cloud-on-k8s/job/cloud-on-k8s-e2e-tests)

# Elastic Cloud on Kubernetes (ECK)

Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch and Kibana on Kubernetes based on the operator pattern.

This is an alpha version.

Current features:

*  Elasticsearch and Kibana deployments
*  TLS Certificates management
*  Safe Elasticsearch cluster configuration & topology changes
*  Persistent volumes usage
*  Custom node configuration and attributes
*  Secure settings keystore updates

Supported versions:

*  Kubernetes: 1.11+
*  Elasticsearch: 6.8+, 7.1+

Check the [Quickstart](https://www.elastic.co/guide/en/cloud-on-k8s/current/index.html) if you want to deploy you first cluster with ECK.

If you want to contribute to the project, check our [contributing guide](CONTRIBUTING.md) and see [how to setup a local development environment](dev-setup.md).
