---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-apm-k8s-elastic-co-v1.html
applies_to:
  deployment:
    eck: all
---

# apm.k8s.elastic.co/v1 [k8s-api-apm-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing APM Server resources.

* [ApmServer](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserver)

## ApmServer [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserver]

ApmServer represents an APM Server resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `apm.k8s.elastic.co/v1`<br> |
| **`kind`** *string*<br> | `ApmServer`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec)*<br> |  |


## ApmServerSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec]

ApmServerSpec holds the specification of an APM Server.

::::{admonition} Appears In:
* [ApmServer](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserver)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of the APM Server.<br> |
| **`image`** *string*<br> | Image is the APM Server Docker image to deploy.<br> |
| **`count`** *integer*<br> | Count of APM Server instances to deploy.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the APM Server configuration. See: [https://www.elastic.co/guide/en/apm/server/current/configuring-howto-apm-server.html](docs-content://solutions/observability/apps/configure-apm-server.md)<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds the HTTP layer configuration for the APM Server resource.<br> |
| **`elasticsearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | ElasticsearchRef is a reference to the output Elasticsearch cluster running in the same Kubernetes cluster.<br> |
| **`kibanaRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster. It allows APM agent central configuration management in Kibana.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the APM Server pods.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |


