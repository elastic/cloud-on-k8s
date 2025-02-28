---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-maps-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# maps.k8s.elastic.co/v1alpha1 [k8s-api-maps-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing Elastic Maps Server resources.

* [ElasticMapsServer](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserver)
* [ElasticMapsServerList](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserverlist)

## ElasticMapsServer [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserver]

ElasticMapsServer represents an Elastic Map Server resource in a Kubernetes cluster.

::::{admonition} Appears In:
* [ElasticMapsServerList](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserverlist)

::::


| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `maps.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `ElasticMapsServer`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[MapsSpec](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec)*<br> |  |


## ElasticMapsServerList [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserverlist]

ElasticMapsServerList contains a list of ElasticMapsServer

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `maps.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `ElasticMapsServerList`<br> |
| **`metadata`** *[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`items`** *[ElasticMapsServer](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserver) array*<br> |  |


## MapsSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec]

MapsSpec holds the specification of an Elastic Maps Server instance.

::::{admonition} Appears In:
* [ElasticMapsServer](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-elasticmapsserver)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of Elastic Maps Server.<br> |
| **`image`** *string*<br> | Image is the Elastic Maps Server Docker image to deploy.<br> |
| **`count`** *integer*<br> | Count of Elastic Maps Server instances to deploy.<br> |
| **`elasticsearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the ElasticMapsServer configuration. See: [docs-content://explore-analyze/visualize/maps/maps-connect-to-ems.md#elastic-maps-server-configuration](docs-content://explore-analyze/visualize/maps/maps-connect-to-ems.md#elastic-maps-server-configuration)<br> |
| **`configRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | ConfigRef contains a reference to an existing Kubernetes Secret holding the Elastic Maps Server configuration. Configuration settings are merged and have precedence over settings specified in `config`.<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds the HTTP layer configuration for Elastic Maps Server.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Elastic Maps Server pods<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |


