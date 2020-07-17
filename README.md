
[![Build Status](https://devops-ci.elastic.co/buildStatus/icon?job=cloud-on-k8s-e2e-tests-master&subject=E2E%20tests)](https://devops-ci.elastic.co/job/cloud-on-k8s-e2e-tests-master/)
[![GitHub release](https://img.shields.io/github/v/release/elastic/cloud-on-k8s.svg)](https://github.com/elastic/cloud-on-k8s/releases/latest)

# Elastic Cloud on Kubernetes (ECK)

Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana and APM Server on Kubernetes based on the operator pattern.

Current features:

*  Elasticsearch, Kibana and APM Server deployments
*  TLS Certificates management
*  Safe Elasticsearch cluster configuration & topology changes
*  Persistent volumes usage
*  Custom node configuration and attributes
*  Secure settings keystore updates

Supported versions:

*  Kubernetes 1.12+ or OpenShift 3.11+
*  Elasticsearch, Kibana, APM Server: 6.8+, 7.1+
*  Enterprise Search: 7.7+
*  Beats: 7.0+

Check the [Quickstart](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-quickstart.html) to deploy your first cluster with ECK.

If you want to contribute to the project, check our [contributing guide](CONTRIBUTING.md) and see [how to setup a local development environment](dev-setup.md).

For general questions, please see the Elastic [forums](https://discuss.elastic.co/c/eck).
