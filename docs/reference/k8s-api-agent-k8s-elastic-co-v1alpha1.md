---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-agent-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# agent.k8s.elastic.co/v1alpha1 [k8s-api-agent-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API Schema definitions for the agent v1alpha1 API group

* [Agent](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agent)

## Agent [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agent]

Agent is the Schema for the Agents API.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `agent.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `Agent`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)*<br> |  |


## AgentMode (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentmode]

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)

::::



## AgentSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec]

AgentSpec defines the desired state of the Agent

::::{admonition} Appears In:
* [Agent](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agent)

::::


| Field | Description |
| --- | --- |
| **`version`** *string*<br> | Version of the Agent.<br> |
| **`elasticsearchRefs`** *[Output](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-output) array*<br> | ElasticsearchRefs is a reference to a list of Elasticsearch clusters running in the same Kubernetes cluster. Due to existing limitations, only a single ES cluster is currently supported.<br> |
| **`image`** *string*<br> | Image is the Agent Docker image to deploy. Version has to match the Agent in the image.<br> |
| **`config`** *[Config](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config)*<br> | Config holds the Agent configuration. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`configRef`** *[ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)*<br> | ConfigRef contains a reference to an existing Kubernetes Secret holding the Agent configuration. Agent settings must be specified as yaml, under a single "agent.yml" entry. At most one of [`Config`, `ConfigRef`] can be specified.<br> |
| **`secureSettings`** *[SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource) array*<br> | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Agent. Secrets data can be then referenced in the Agent config using the Secret’s keys or as specified in `Entries` field of each SecureSetting.<br> |
| **`serviceAccountName`** *string*<br> | ServiceAccountName is used to check access from the current resource to an Elasticsearch resource in a different namespace. Can only be used if ECK is enforcing RBAC on references.<br> |
| **`daemonSet`** *[DaemonSetSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-daemonsetspec)*<br> | DaemonSet specifies the Agent should be deployed as a DaemonSet, and allows providing its spec. Cannot be used along with `deployment` or `statefulSet`.<br> |
| **`deployment`** *[DeploymentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-deploymentspec)*<br> | Deployment specifies the Agent should be deployed as a Deployment, and allows providing its spec. Cannot be used along with `daemonSet` or `statefulSet`.<br> |
| **`statefulSet`** *[StatefulSetSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-statefulsetspec)*<br> | StatefulSet specifies the Agent should be deployed as a StatefulSet, and allows providing its spec. Cannot be used along with `daemonSet` or `deployment`.<br> |
| **`revisionHistoryLimit`** *integer*<br> | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying DaemonSet or Deployment or StatefulSet.<br> |
| **`http`** *[HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)*<br> | HTTP holds the HTTP layer configuration for the Agent in Fleet mode with Fleet Server enabled.<br> |
| **`mode`** *[AgentMode](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentmode)*<br> | Mode specifies the source of configuration for the Agent. The configuration can be specified locally through `config` or `configRef` (`standalone` mode), or come from Fleet during runtime (`fleet` mode). Defaults to `standalone` mode.<br> |
| **`fleetServerEnabled`** *boolean*<br> | FleetServerEnabled determines whether this Agent will launch Fleet Server. Don’t set unless `mode` is set to `fleet`.<br> |
| **`policyID`** *string*<br> | PolicyID determines into which Agent Policy this Agent will be enrolled. This field will become mandatory in a future release, default policies are deprecated since 8.1.0.<br> |
| **`kibanaRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | KibanaRef is a reference to Kibana where Fleet should be set up and this Agent should be enrolled. Don’t set unless `mode` is set to `fleet`.<br> |
| **`fleetServerRef`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> | FleetServerRef is a reference to Fleet Server that this Agent should connect to to obtain it’s configuration. Don’t set unless `mode` is set to `fleet`.<br> |


## DaemonSetSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-daemonsetspec]

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)

::::


| Field | Description |
| --- | --- |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> |  |
| **`updateStrategy`** *[DaemonSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#daemonsetupdatestrategy-v1-apps)*<br> |  |


## DeploymentSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-deploymentspec]

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)

::::


| Field | Description |
| --- | --- |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> |  |
| **`replicas`** *integer*<br> |  |
| **`strategy`** *[DeploymentStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#deploymentstrategy-v1-apps)*<br> |  |


## Output [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-output]

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)

::::


| Field | Description |
| --- | --- |
| **`ObjectSelector`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector)*<br> |  |
| **`outputName`** *string*<br> |  |


## StatefulSetSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-statefulsetspec]

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)

::::


| Field | Description |
| --- | --- |
| **`podTemplate`** *[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)*<br> |  |
| **`replicas`** *integer*<br> |  |
| **`serviceName`** *string*<br> |  |
| **`podManagementPolicy`** *[PodManagementPolicyType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podmanagementpolicytype-v1-apps)*<br> | PodManagementPolicy controls how pods are created during initial scale up, when replacing pods on nodes, or when scaling down. The default policy is `Parallel`, where pods are created in parallel to match the desired scale without waiting, and on scale down will delete all pods at once. The alternative policy is `OrderedReady`, the default for vanilla kubernetes StatefulSets, where pods are created in increasing order in increasing order (pod-0, then pod-1, etc.) and the controller will wait until each pod is ready before continuing. When scaling down, the pods are removed in the opposite order.<br> |
| **`volumeClaimTemplates`** *[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array*<br> | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod. Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate. Items defined here take precedence over any default claims added by the operator with the same name.<br> |


