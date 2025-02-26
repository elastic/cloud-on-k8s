---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-logstash-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# logstash.k8s.elastic.co/v1alpha1 [k8s-api-logstash-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API Schema definitions for the logstash v1alpha1 API group

* [Logstash](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstash)

## ElasticsearchCluster [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-elasticsearchcluster]

ElasticsearchCluster is a named reference to an Elasticsearch cluster which can be used in a Logstash pipeline.

::::{admonition} Appears In:
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)

::::


| Field | Description |
| --- | --- |
| **`ObjectSelector`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> |  |
| **`clusterName`** *string*<br> | ClusterName is an alias for the cluster to be used to refer to the Elasticsearch cluster in Logstash configuration files, and will be used to identify "named clusters" in Logstash<br> |


## Logstash [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstash]

Logstash is the Schema for the logstashes API

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `logstash.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `Logstash`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)*<br> |  |
| **`status`** *[LogstashStatus](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashstatus)*<br> |  |


## LogstashHealth (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashhealth]

::::{admonition} Appears In:
* [LogstashStatus](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashstatus)

::::



## LogstashService [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashservice]

::::{admonition} Appears In:
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> |  |
| **`service`** *[ServiceTemplate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-servicetemplate)*<br> | Service defines the template for the associated Kubernetes Service object.<br> |
| **`tls`** *[TLSOptions](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-tlsoptions)*<br> | TLS defines options for configuring TLS for HTTP.<br> |


## LogstashSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec]

LogstashSpec defines the desired state of Logstash

::::{admonition} Appears In:
* [Logstash](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstash)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of the Logstash.<br> |
| **`count`** *integer*<br> |  |
| **`image`** *string*<br> | Image is the Logstash Docker image to deploy. Version and Type have to match the Logstash in the image.<br> |
| **`elasticsearchRefs`** *[ElasticsearchCluster](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-elasticsearchcluster) array*<br> | ElasticsearchRefs are references to Elasticsearch clusters running in the same Kubernetes cluster.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Logstash configuration. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`configRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | ConfigRef contains a reference to an existing Kubernetes Secret holding the Logstash configuration. Logstash settings must be specified as yaml, under a single "logstash.yml" entry. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`pipelines`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config) array*<br> | Pipelines holds the Logstash Pipelines. At most one of [`Pipelines`, `PipelinesRef`] can be specified.<br> |
| **`pipelinesRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | PipelinesRef contains a reference to an existing Kubernetes Secret holding the Logstash Pipelines. Logstash pipelines must be specified as yaml, under a single "pipelines.yml" entry. At most one of [`Pipelines`, `PipelinesRef`] can be specified.<br> |
| **`services`** *[LogstashService](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashservice) array*<br> | Services contains details of services that Logstash should expose - similar to the HTTP layer configuration for the rest of the stack, but also applicable for more use cases than the metrics API, as logstash may need to be opened up for other services: Beats, TCP, UDP, etc, inputs.<br> |
| **`monitoring`** *[Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)*<br> | Monitoring enables you to collect and ship log and monitoring data of this Logstash. Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different Elasticsearch monitoring clusters running in the same Kubernetes cluster.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options for the Logstash pods.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSet.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Logstash. Secrets data can be then referenced in the Logstash config using the Secret’s keys or as specified in `Entries` field of each SecureSetting.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |
| **`updateStrategy`** *[StatefulSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#statefulsetupdatestrategy-v1-apps)*<br> | UpdateStrategy is a StatefulSetUpdateStrategy. The default type is "RollingUpdate".<br> |
| **`volumeClaimTemplates`** *[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array*<br> | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod. Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate. Items defined here take precedence over any default claims added by the operator with the same name.<br> |


## LogstashStatus [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashstatus]

LogstashStatus defines the observed state of Logstash

::::{admonition} Appears In:
* [Logstash](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstash)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of the stack resource currently running. During version upgrades, multiple versions may run in parallel: this value specifies the lowest version currently running.<br> |
| **`expectedNodes`** *integer*<br> |  |
| **`availableNodes`** *integer*<br> |  |
| **`health`** *[LogstashHealth](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashhealth)*<br> |  |
| **`observedGeneration`** *integer*<br> | ObservedGeneration is the most recent generation observed for this Logstash instance. It corresponds to the metadata generation, which is updated on mutation by the API Server. If the generation observed in status diverges from the generation in metadata, the Logstash controller has not yet processed the changes contained in the Logstash specification.<br> |
| **`selector`** *string*<br> |  |


