---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# stackconfigpolicy.k8s.elastic.co/v1alpha1 [k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing StackConfigPolicy resources.

* [StackConfigPolicy](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy)

## ElasticsearchConfigPolicySpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec]

::::{admonition} Appears In:
* [StackConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

::::


| Field | Description |
| --- | --- |
| **`clusterSettings`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | ClusterSettings holds the Elasticsearch cluster settings (/_cluster/settings)<br> |
| **`snapshotRepositories`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | SnapshotRepositories holds the Snapshot Repositories settings (/_snapshot)<br> |
| **`snapshotLifecyclePolicies`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | SnapshotLifecyclePolicies holds the Snapshot Lifecycle Policies settings (/_slm/policy)<br> |
| **`securityRoleMappings`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | SecurityRoleMappings holds the Role Mappings settings (/_security/role_mapping)<br> |
| **`indexLifecyclePolicies`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | IndexLifecyclePolicies holds the Index Lifecycle policies settings (/_ilm/policy)<br> |
| **`ingestPipelines`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | IngestPipelines holds the Ingest Pipelines settings (/_ingest/pipeline)<br> |
| **`indexTemplates`** *[IndexTemplates](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates)*<br> | IndexTemplates holds the Index and Component Templates settings<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the settings that go into elasticsearch.yml.<br> |
| **`secretMounts`** *[SecretMount](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-secretmount) array*<br> | SecretMounts are additional Secrets that need to be mounted into the Elasticsearch pods.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings are additional Secrets that contain data to be configured to Elasticsearch’s keystore.<br> |


## IndexTemplates [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates]

::::{admonition} Appears In:
* [ElasticsearchConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)

::::


| Field | Description |
| --- | --- |
| **`componentTemplates`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | ComponentTemplates holds the Component Templates settings (/_component_template)<br> |
| **`composableIndexTemplates`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | ComposableIndexTemplates holds the Index Templates settings (/_index_template)<br> |


## KibanaConfigPolicySpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec]

::::{admonition} Appears In:
* [StackConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

::::


| Field | Description |
| --- | --- |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the settings that go into kibana.yml.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings are additional Secrets that contain data to be configured to Kibana’s keystore.<br> |


## SecretMount [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-secretmount]

SecretMount contains information about additional secrets to be mounted to the elasticsearch pods

::::{admonition} Appears In:
* [ElasticsearchConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName denotes the name of the secret that needs to be mounted to the elasticsearch pod<br> |
| **`mountPath`** *string*<br> | MountPath denotes the path to which the secret should be mounted to inside the elasticsearch pod<br> |


## StackConfigPolicy [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy]

StackConfigPolicy represents a StackConfigPolicy resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `stackconfigpolicy.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `StackConfigPolicy`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[StackConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)*<br> |  |


## StackConfigPolicySpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec]

::::{admonition} Appears In:
* [StackConfigPolicy](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy)

::::


| Field | Description |
| --- | --- |
| **`resourceSelector`** *[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta)*<br> |  |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | Deprecated: SecureSettings only applies to Elasticsearch and is deprecated. It must be set per application instead.<br> |
| **`elasticsearch`** *[ElasticsearchConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)*<br> |  |
| **`kibana`** *[KibanaConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)*<br> |  |


