---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-enterprisesearch-k8s-elastic-co-v1.html
applies_to:
  deployment:
    eck: all
---

# enterprisesearch.k8s.elastic.co/v1 [k8s-api-enterprisesearch-k8s-elastic-co-v1]

Package v1beta1 contains API schema definitions for managing Enterprise Search resources.

* [EnterpriseSearch](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearch)

## EnterpriseSearch [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearch]

EnterpriseSearch is a Kubernetes CRD to represent Enterprise Search.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `enterprisesearch.k8s.elastic.co/v1`<br> |
| **`kind`** *string*<br> | `EnterpriseSearch`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec)*<br> |  |


## EnterpriseSearchSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec]

EnterpriseSearchSpec holds the specification of an Enterprise Search resource.

::::{admonition} Appears In:
* [EnterpriseSearch](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearch)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of Enterprise Search.<br> |
| **`image`** *string*<br> | Image is the Enterprise Search Docker image to deploy.<br> |
| **`count`** *integer*<br> | Count of Enterprise Search instances to deploy.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Enterprise Search configuration.<br> |
| **`configRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | ConfigRef contains a reference to an existing Kubernetes Secret holding the Enterprise Search configuration. Configuration settings are merged and have precedence over settings specified in `config`.<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds the HTTP layer configuration for Enterprise Search resource.<br> |
| **`elasticsearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | ElasticsearchRef is a reference to the Elasticsearch cluster running in the same Kubernetes cluster.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Enterprise Search pods.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |


