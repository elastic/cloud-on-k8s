---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-beat-k8s-elastic-co-v1beta1.html
applies_to:
  deployment:
    eck: all
---

# beat.k8s.elastic.co/v1beta1 [k8s-api-beat-k8s-elastic-co-v1beta1]

Package v1beta1 contains API Schema definitions for the beat v1beta1 API group

* [Beat](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beat)

## Beat [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beat]

Beat is the Schema for the Beats API.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `beat.k8s.elastic.co/v1beta1`<br> |
| **`kind`** *string*<br> | `Beat`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)*<br> |  |


## BeatSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec]

BeatSpec defines the desired state of a Beat.

::::{admonition} Appears In:
* [Beat](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beat)

::::


| Field | Description |
| --- | --- |
| **`type`** *string*<br> | Type is the type of the Beat to deploy (filebeat, metricbeat, heartbeat, auditbeat, journalbeat, packetbeat, and so on). Any string can be used, but well-known types will have the image field defaulted and have the appropriate Elasticsearch roles created automatically. It also allows for dashboard setup when combined with a `KibanaRef`.<br> |
| **`version`** *string*<br> | Version of the Beat.<br> |
| **`elasticsearchRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster.<br> |
| **`kibanaRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster. It allows automatic setup of dashboards and visualizations.<br> |
| **`image`** *string*<br> | Image is the Beat Docker image to deploy. Version and Type have to match the Beat in the image.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Beat configuration. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`configRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | ConfigRef contains a reference to an existing Kubernetes Secret holding the Beat configuration. Beat settings must be specified as yaml, under a single "beat.yml" entry. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Beat. Secrets data can be then referenced in the Beat config using the Secret’s keys or as specified in `Entries` field of each SecureSetting.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |
| **`daemonSet`** *[DaemonSetSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-daemonsetspec)*<br> | DaemonSet specifies the Beat should be deployed as a DaemonSet, and allows providing its spec. Cannot be used along with `deployment`. If both are absent a default for the Type is used.<br> |
| **`deployment`** *[DeploymentSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-deploymentspec)*<br> | Deployment specifies the Beat should be deployed as a Deployment, and allows providing its spec. Cannot be used along with `daemonSet`. If both are absent a default for the Type is used.<br> |
| **`monitoring`** *[Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)*<br> | Monitoring enables you to collect and ship logs and metrics for this Beat. Metricbeat and/or Filebeat sidecars are configured and send monitoring data to an Elasticsearch monitoring cluster running in the same Kubernetes cluster.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying DaemonSet or Deployment.<br> |


## DaemonSetSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-daemonsetspec]

::::{admonition} Appears In:
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)

::::


| Field | Description |
| --- | --- |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> |  |
| **`updateStrategy`** *[DaemonSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#daemonsetupdatestrategy-v1-apps)*<br> |  |


## DeploymentSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-deploymentspec]

::::{admonition} Appears In:
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)

::::


| Field | Description |
| --- | --- |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> |  |
| **`replicas`** *integer*<br> |  |
| **`strategy`** *[DeploymentStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#deploymentstrategy-v1-apps)*<br> |  |


