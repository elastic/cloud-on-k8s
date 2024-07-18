
[![Build status](https://badge.buildkite.com/8fe262ce6fc1da017fc91c35465c1fe0addbc94c38afc9f04b.svg?branch=main)](https://buildkite.com/elastic/cloud-on-k8s-operator)
[![GitHub release](https://img.shields.io/github/v/release/elastic/cloud-on-k8s.svg)](https://github.com/elastic/cloud-on-k8s/releases/latest)

# Elastic Cloud on Kubernetes (ECK)

Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, APM Server, Enterprise Search, Beats, Elastic Agent, Elastic Maps Server, and Logstash on Kubernetes based on the operator pattern.

Current features:

*  Elasticsearch, Kibana, APM Server, Enterprise Search, and Beats deployments
*  TLS Certificates management
*  Safe Elasticsearch cluster configuration & topology changes
*  Persistent volumes usage
*  Custom node configuration and attributes
*  Secure settings keystore updates

Supported versions:

*  Kubernetes 1.26-1.30
*  OpenShift 4.12-4.16
*  Elasticsearch, Kibana, APM Server: 6.8+, 7.1+, 8+
*  Enterprise Search: 7.7+, 8+
*  Beats: 7.0+, 8+
*  Elastic Agent: 7.10+ (standalone), 7.14+, 8+ (Fleet)
*  Elastic Maps Server: 7.11+, 8+
*  Logstash 8.7+

Check the [Quickstart](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-quickstart.html) to deploy your first cluster with ECK.

If you want to contribute to the project, check our [contributing guide](CONTRIBUTING.md) and see [how to setup a local development environment](dev-setup.md).

For general questions, please see the Elastic [forums](https://discuss.elastic.co/c/eck).
