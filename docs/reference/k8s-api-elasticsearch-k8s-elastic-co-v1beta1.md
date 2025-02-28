---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-elasticsearch-k8s-elastic-co-v1beta1.html
applies_to:
  deployment:
    eck: all
---

# elasticsearch.k8s.elastic.co/v1beta1 [k8s-api-elasticsearch-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for managing Elasticsearch resources.

* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearch)

## ChangeBudget [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-changebudget]

ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.

::::{admonition} Appears In:
* [UpdateStrategy](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-updatestrategy)

::::


| Field | Description |
| --- | --- |
| **`maxUnavailable`** *integer*<br> | MaxUnavailable is the maximum number of pods that can be unavailable (not ready) during the update due to circumstances under the control of the operator. Setting a negative value will disable this restriction. Defaults to 1 if not specified.<br> |
| **`maxSurge`** *integer*<br> | MaxSurge is the maximum number of new pods that can be created exceeding the original number of pods defined in the specification. MaxSurge is only taken into consideration when scaling up. Setting a negative value will disable the restriction. Defaults to unbounded if not specified.<br> |


## Elasticsearch [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearch]

Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `elasticsearch.k8s.elastic.co/v1beta1`<br> |
| **`kind`** *string*<br> | `Elasticsearch`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)*<br> |  |
| **`status`** *[ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)*<br> |  |


## ElasticsearchHealth (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchhealth]

ElasticsearchHealth is the health of the cluster as returned by the health API.

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)

::::



## ElasticsearchOrchestrationPhase (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchorchestrationphase]

ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)

::::



## ElasticsearchSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec]

ElasticsearchSpec holds the specification of an Elasticsearch cluster.

::::{admonition} Appears In:
* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearch)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of Elasticsearch.<br> |
| **`image`** *string*<br> | Image is the Elasticsearch Docker image to deploy.<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-httpconfig)*<br> | HTTP holds HTTP layer settings for Elasticsearch.<br> |
| **`nodeSets`** *[NodeSet](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-nodeset) array*<br> | NodeSets allow specifying groups of Elasticsearch nodes sharing the same configuration and Pod templates.<br> |
| **`updateStrategy`** *[UpdateStrategy](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-updatestrategy)*<br> | UpdateStrategy specifies how updates to the cluster should be performed.<br> |
| **`podDisruptionBudget`** *[PodDisruptionBudgetTemplate](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-poddisruptionbudgettemplate)*<br> | PodDisruptionBudget provides access to the default pod disruption budget for the Elasticsearch cluster. The default budget selects all cluster pods and sets `maxUnavailable` to 1. To disable, set `PodDisruptionBudget` to the empty value (`{}` in YAML).<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Elasticsearch.<br> |


## ElasticsearchStatus [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus]

ElasticsearchStatus defines the observed state of Elasticsearch

::::{admonition} Appears In:
* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearch)

::::


| Field | Description |
| --- | --- |
| **`health`** *[ElasticsearchHealth](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchhealth)*<br> |  |
| **`phase`** *[ElasticsearchOrchestrationPhase](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchorchestrationphase)*<br> |  |


## NodeSet [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-nodeset]

NodeSet is the specification for a group of Elasticsearch nodes sharing the same configuration and a Pod template.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of this set of nodes. Becomes a part of the Elasticsearch node.name setting.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-config)*<br> | Config holds the Elasticsearch configuration.<br> |
| **`count`** *integer*<br> | Count of Elasticsearch nodes to deploy.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet.<br> |
| **`volumeClaimTemplates`** *[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array*<br> | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet. Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate. Items defined here take precedence over any default claims added by the operator with the same name.<br> |


## UpdateStrategy [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-updatestrategy]

UpdateStrategy specifies how updates to the cluster should be performed.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`changeBudget`** *[ChangeBudget](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-changebudget)*<br> | ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.<br> |


