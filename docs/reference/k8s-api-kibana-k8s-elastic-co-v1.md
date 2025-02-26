---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-kibana-k8s-elastic-co-v1.html
applies_to:
  deployment:
    eck: all
---

# kibana.k8s.elastic.co/v1 [k8s-api-kibana-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing Kibana resources.

* [Kibana](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibana)

## Kibana [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibana]

Kibana represents a Kibana resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `kibana.k8s.elastic.co/v1`<br> |
| **`kind`** *string*<br> | `Kibana`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)*<br> |  |


## KibanaSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec]

KibanaSpec holds the specification of a Kibana instance.

::::{admonition} Appears In:
* [Kibana](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibana)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of Kibana.<br> |
| **`image`** *string*<br> | Image is the Kibana Docker image to deploy.<br> |
| **`count`** *integer*<br> | Count of Kibana instances to deploy.<br> |
| **`elasticsearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.<br> |
| **`enterpriseSearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | EnterpriseSearchRef is a reference to an EnterpriseSearch running in the same Kubernetes cluster. Kibana provides the default Enterprise Search UI starting version 7.14.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Kibana configuration. See: [/kibana/docs/reference/configuration-reference/general-settings.md](kibana://docs/reference/configuration-reference/general-settings.md)<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds the HTTP layer configuration for Kibana.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |
| **`monitoring`** *[Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)*<br> | Monitoring enables you to collect and ship log and monitoring data of this Kibana. See [docs-content://deploy-manage/monitor/monitoring-data/visualizing-monitoring-data.md](docs-content://deploy-manage/monitor/monitoring-data/visualizing-monitoring-data.md). Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different Elasticsearch monitoring clusters running in the same Kubernetes cluster.<br> |


