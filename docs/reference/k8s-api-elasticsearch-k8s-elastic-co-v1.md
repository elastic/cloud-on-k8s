---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-elasticsearch-k8s-elastic-co-v1.html
applies_to:
  deployment:
    eck: all
---

# elasticsearch.k8s.elastic.co/v1 [k8s-api-elasticsearch-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing Elasticsearch resources.

* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearch)

## Auth [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-auth]

Auth contains user authentication and authorization security settings for Elasticsearch.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`roles`** *[RoleSource](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-rolesource) array*<br> | Roles to propagate to the Elasticsearch cluster.<br> |
| **`fileRealm`** *[FileRealmSource](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-filerealmsource) array*<br> | FileRealm to propagate to the Elasticsearch cluster.<br> |
| **`disableElasticUser`** *boolean*<br> | DisableElasticUser disables the default elastic user that is created by ECK.<br> |


## ChangeBudget [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-changebudget]

ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.

::::{admonition} Appears In:
* [UpdateStrategy](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-updatestrategy)

::::


| Field | Description |
| --- | --- |
| **`maxUnavailable`** *integer*<br> | MaxUnavailable is the maximum number of Pods that can be unavailable (not ready) during the update due to circumstances under the control of the operator. Setting a negative value will disable this restriction. Defaults to 1 if not specified.<br> |
| **`maxSurge`** *integer*<br> | MaxSurge is the maximum number of new Pods that can be created exceeding the original number of Pods defined in the specification. MaxSurge is only taken into consideration when scaling up. Setting a negative value will disable the restriction. Defaults to unbounded if not specified.<br> |


## DownscaleOperation [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-downscaleoperation]

DownscaleOperation provides details about in progress downscale operations. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [InProgressOperations](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-inprogressoperations)

::::


| Field | Description |
| --- | --- |
| **`lastUpdatedTime`** *[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)*<br> |  |
| **`nodes`** *[DownscaledNode](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-downscalednode) array*<br> | Nodes which are scheduled to be removed from the cluster.<br> |
| **`stalled`** *boolean*<br> | Stalled represents a state where no progress can be made. It is only available for clusters managed with the Elasticsearch shutdown API.<br> |


## DownscaledNode [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-downscalednode]

DownscaledNode provides an overview of in progress changes applied by the operator to remove Elasticsearch nodes from the cluster. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [DownscaleOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-downscaleoperation)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of the Elasticsearch node that should be removed.<br> |
| **`shutdownStatus`** *string*<br> | Shutdown status as returned by the Elasticsearch shutdown API. If the Elasticsearch shutdown API is not available, the shutdown status is then inferred from the remaining shards on the nodes, as observed by the operator.<br> |
| **`explanation`** *string*<br> | Explanation provides details about an in progress node shutdown. It is only available for clusters managed with the Elasticsearch shutdown API.<br> |


## Elasticsearch [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearch]

Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `elasticsearch.k8s.elastic.co/v1`<br> |
| **`kind`** *string*<br> | `Elasticsearch`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)*<br> |  |
| **`status`** *[ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus)*<br> |  |


## ElasticsearchHealth (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchhealth]

ElasticsearchHealth is the health of the cluster as returned by the health API.

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus)

::::



## ElasticsearchOrchestrationPhase (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchorchestrationphase]

ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus)

::::



## ElasticsearchSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec]

ElasticsearchSpec holds the specification of an Elasticsearch cluster.

::::{admonition} Appears In:
* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearch)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of Elasticsearch.<br> |
| **`image`** *string*<br> | Image is the Elasticsearch Docker image to deploy.<br> |
| **`remoteClusterServer`** *[RemoteClusterServer](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusterserver)*<br> | RemoteClusterServer specifies if the remote cluster server should be enabled. This must be enabled if this cluster is a remote cluster which is expected to be accessed using API key authentication.<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds HTTP layer settings for Elasticsearch.<br> |
| **`transport`** *[TransportConfig](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transportconfig)*<br> | Transport holds transport layer settings for Elasticsearch.<br> |
| **`nodeSets`** *[NodeSet](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-nodeset) array*<br> | NodeSets allow specifying groups of Elasticsearch nodes sharing the same configuration and Pod templates.<br> |
| **`updateStrategy`** *[UpdateStrategy](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-updatestrategy)*<br> | UpdateStrategy specifies how updates to the cluster should be performed.<br> |
| **`podDisruptionBudget`** *[PodDisruptionBudgetTemplate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-poddisruptionbudgettemplate)*<br> | PodDisruptionBudget provides access to the default Pod disruption budget for the Elasticsearch cluster. The default budget doesn’t allow any Pod to be removed in case the cluster is not green or if there is only one node of type `data` or `master`. In all other cases the default PodDisruptionBudget sets `minUnavailable` equal to the total number of nodes minus 1. To disable, set `PodDisruptionBudget` to the empty value (`{}` in YAML).<br> |
| **`auth`** *[Auth](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-auth)*<br> | Auth contains user authentication and authorization security settings for Elasticsearch.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Elasticsearch.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to a resource (for ex. a remote Elasticsearch cluster) in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |
| **`remoteClusters`** *[RemoteCluster](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remotecluster) array*<br> | RemoteClusters enables you to establish uni-directional connections to a remote Elasticsearch cluster.<br> |
| **`volumeClaimDeletePolicy`** *[VolumeClaimDeletePolicy](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-volumeclaimdeletepolicy)*<br> | VolumeClaimDeletePolicy sets the policy for handling deletion of PersistentVolumeClaims for all NodeSets. Possible values are DeleteOnScaledownOnly and DeleteOnScaledownAndClusterDeletion. Defaults to DeleteOnScaledownAndClusterDeletion.<br> |
| **`monitoring`** *[Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)*<br> | Monitoring enables you to collect and ship log and monitoring data of this Elasticsearch cluster. See [docs-content://deploy-manage/monitor.md](docs-content://deploy-manage/monitor.md). Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different Elasticsearch monitoring clusters running in the same Kubernetes cluster.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSets.<br> |


## ElasticsearchStatus [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus]

ElasticsearchStatus represents the observed state of Elasticsearch.

::::{admonition} Appears In:
* [Elasticsearch](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearch)

::::


| Field | Description |
| --- | --- |
| **`availableNodes`** *integer*<br> | AvailableNodes is the number of available instances.<br> |
| **`version`** *string*<br> | Version of the stack resource currently running. During version upgrades, multiple versions may run in parallel: this value specifies the lowest version currently running.<br> |
| **`health`** *[ElasticsearchHealth](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchhealth)*<br> |  |
| **`phase`** *[ElasticsearchOrchestrationPhase](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchorchestrationphase)*<br> |  |
| **`conditions`** *[Conditions](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditions)*<br> | Conditions holds the current service state of an Elasticsearch cluster. **This API is in technical preview and may be changed or removed in a future release.**<br> |
| **`inProgressOperations`** *[InProgressOperations](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-inprogressoperations)*<br> | InProgressOperations represents changes being applied by the operator to the Elasticsearch cluster. **This API is in technical preview and may be changed or removed in a future release.**<br> |
| **`observedGeneration`** *integer*<br> | ObservedGeneration is the most recent generation observed for this Elasticsearch cluster. It corresponds to the metadata generation, which is updated on mutation by the API Server. If the generation observed in status diverges from the generation in metadata, the Elasticsearch controller has not yet processed the changes contained in the Elasticsearch specification.<br> |


## FieldSecurity [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-fieldsecurity]

::::{admonition} Appears In:
* [Search](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-search)

::::


| Field | Description |
| --- | --- |
| **`grant`** *string array*<br> |  |
| **`except`** *string array*<br> |  |


## FileRealmSource [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-filerealmsource]

FileRealmSource references users to create in the Elasticsearch cluster.

::::{admonition} Appears In:
* [Auth](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-auth)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |


## InProgressOperations [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-inprogressoperations]

InProgressOperations provides details about in progress changes applied by the operator on the Elasticsearch cluster. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus)

::::


| Field | Description |
| --- | --- |
| **`downscale`** *[DownscaleOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-downscaleoperation)*<br> |  |
| **`upgrade`** *[UpgradeOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upgradeoperation)*<br> |  |
| **`upscale`** *[UpscaleOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upscaleoperation)*<br> |  |


## NewNode [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-newnode]

::::{admonition} Appears In:
* [UpscaleOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upscaleoperation)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of the Elasticsearch node that should be added to the cluster.<br> |
| **`status`** *[NewNodeStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-newnodestatus)*<br> | NewNodeStatus states if a new node is being created, or if the upscale is delayed.<br> |
| **`message`** *string*<br> | Optional message to explain why a node may not be immediately added.<br> |


## NewNodeStatus (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-newnodestatus]

NewNodeStatus provides details about the status of nodes which are expected to be created and added to the Elasticsearch cluster. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [NewNode](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-newnode)

::::



## NodeSet [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-nodeset]

NodeSet is the specification for a group of Elasticsearch nodes sharing the same configuration and a Pod template.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of this set of nodes. Becomes a part of the Elasticsearch node.name setting.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Elasticsearch configuration.<br> |
| **`count`** *integer*<br> | Count of Elasticsearch nodes to deploy. If the node set is managed by an autoscaling policy the initial value is automatically set by the autoscaling controller.<br> |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet.<br> |
| **`volumeClaimTemplates`** *[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array*<br> | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet. Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate. Items defined here take precedence over any default claims added by the operator with the same name.<br> |


## RemoteCluster [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remotecluster]

RemoteCluster declares a remote Elasticsearch cluster connection.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name is the name of the remote cluster as it is set in the Elasticsearch settings. The name is expected to be unique for each remote clusters.<br> |
| **`elasticsearchRef`** *[LocalObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-localobjectselector)*<br> | ElasticsearchRef is a reference to an Elasticsearch cluster running within the same k8s cluster.<br> |
| **`apiKey`** *[RemoteClusterAPIKey](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusterapikey)*<br> | APIKey can be used to enable remote cluster access using Cross-Cluster API keys: [https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-security-create-cross-cluster-api-key](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-security-create-cross-cluster-api-key)<br> |


## RemoteClusterAPIKey [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusterapikey]

RemoteClusterAPIKey defines a remote cluster API Key.

::::{admonition} Appears In:
* [RemoteCluster](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remotecluster)

::::


| Field | Description |
| --- | --- |
| **`access`** *[RemoteClusterAccess](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusteraccess)*<br> | Access is the name of the API Key. It is automatically generated if not set or empty.<br> |


## RemoteClusterAccess [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusteraccess]

RemoteClusterAccess models the API key specification as documented in [https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-security-create-cross-cluster-api-key](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-security-create-cross-cluster-api-key)

::::{admonition} Appears In:
* [RemoteClusterAPIKey](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusterapikey)

::::


| Field | Description |
| --- | --- |
| **`search`** *[Search](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-search)*<br> |  |
| **`replication`** *[Replication](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-replication)*<br> |  |


## RemoteClusterServer [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusterserver]

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`enabled`** *boolean*<br> |  |


## Replication [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-replication]

::::{admonition} Appears In:
* [RemoteClusterAccess](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusteraccess)

::::


| Field | Description |
| --- | --- |
| **`names`** *string array*<br> |  |


## RoleSource [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-rolesource]

RoleSource references roles to create in the Elasticsearch cluster.

::::{admonition} Appears In:
* [Auth](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-auth)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |


## Search [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-search]

::::{admonition} Appears In:
* [RemoteClusterAccess](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remoteclusteraccess)

::::


| Field | Description |
| --- | --- |
| **`names`** *string array*<br> |  |
| **`field_security`** *[FieldSecurity](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-fieldsecurity)*<br> |  |
| **`query`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> |  |
| **`allow_restricted_indices`** *boolean*<br> |  |


## SelfSignedTransportCertificates [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-selfsignedtransportcertificates]

SelfSignedTransportCertificates holds configuration for the self-signed certificates generated by the operator.

::::{admonition} Appears In:
* [TransportTLSOptions](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions)

::::


| Field | Description |
| --- | --- |
| **`disabled`** *boolean*<br> | Disabled indicates that provisioning of the self-signed certificates should be disabled.<br> |


## TransportConfig [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transportconfig]

TransportConfig holds the transport layer settings for Elasticsearch.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`service`** *[ServiceTemplate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-servicetemplate)*<br> | Service defines the template for the associated Kubernetes Service object.<br> |
| **`tls`** *[TransportTLSOptions](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions)*<br> | TLS defines options for configuring TLS on the transport layer.<br> |


## TransportTLSOptions [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions]

::::{admonition} Appears In:
* [TransportConfig](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transportconfig)

::::


| Field | Description |
| --- | --- |
| **`otherNameSuffix`** *string*<br> | OtherNameSuffix when defined will be prefixed with the Pod name and used as the common name, and the first DNSName, as well as an OtherName required by Elasticsearch in the Subject Alternative Name extension of each Elasticsearch node’s transport TLS certificate. Example: if set to "node.cluster.local", the generated certificate will have its otherName set to "<pod_name>.node.cluster.local".<br> |
| **`subjectAltNames`** *[SubjectAlternativeName](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-subjectalternativename) array*<br> | SubjectAlternativeNames is a list of SANs to include in the generated node transport TLS certificates.<br> |
| **`certificate`** *[SecretRef](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretref)*<br> | Certificate is a reference to a Kubernetes secret that contains the CA certificate and private key for generating node certificates. The referenced secret should contain the following:<br><br>* `ca.crt`: The CA certificate in PEM format.<br>* `ca.key`: The private key for the CA certificate in PEM format.<br> |
| **`certificateAuthorities`** *[ConfigMapRef](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configmapref)*<br> | CertificateAuthorities is a reference to a config map that contains one or more x509 certificates for trusted authorities in PEM format. The certificates need to be in a file called `ca.crt`.<br> |
| **`selfSignedCertificates`** *[SelfSignedTransportCertificates](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-selfsignedtransportcertificates)*<br> | SelfSignedCertificates allows configuring the self-signed certificate generated by the operator.<br> |


## UpdateStrategy [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-updatestrategy]

UpdateStrategy specifies how updates to the cluster should be performed.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`changeBudget`** *[ChangeBudget](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-changebudget)*<br> | ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.<br> |


## UpgradeOperation [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upgradeoperation]

UpgradeOperation provides an overview of the pending or in progress changes applied by the operator to update the Elasticsearch nodes in the cluster. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [InProgressOperations](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-inprogressoperations)

::::


| Field | Description |
| --- | --- |
| **`lastUpdatedTime`** *[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)*<br> |  |
| **`nodes`** *[UpgradedNode](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upgradednode) array*<br> | Nodes that must be restarted for upgrade.<br> |


## UpgradedNode [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upgradednode]

UpgradedNode provides details about the status of nodes which are expected to be updated. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [UpgradeOperation](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upgradeoperation)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of the Elasticsearch node that should be upgraded.<br> |
| **`status`** *string*<br> | Status states if the node is either in the process of being deleted for an upgrade, or blocked by a predicate or another condition stated in the message field.<br> |
| **`message`** *string*<br> | Optional message to explain why a node may not be immediately restarted for upgrade.<br> |
| **`predicate`** *string*<br> | Predicate is the name of the predicate currently preventing this node from being deleted for an upgrade.<br> |


## UpscaleOperation [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-upscaleoperation]

UpscaleOperation provides an overview of in progress changes applied by the operator to add Elasticsearch nodes to the cluster. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [InProgressOperations](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-inprogressoperations)

::::


| Field | Description |
| --- | --- |
| **`lastUpdatedTime`** *[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)*<br> |  |
| **`nodes`** *[NewNode](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-newnode) array*<br> | Nodes expected to be added by the operator.<br> |


## VolumeClaimDeletePolicy (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-volumeclaimdeletepolicy]

VolumeClaimDeletePolicy describes the delete policy for handling PersistentVolumeClaims that hold Elasticsearch data. Inspired by [https://github.com/kubernetes/enhancements/pull/2440](https://github.com/kubernetes/enhancements/pull/2440)

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::
