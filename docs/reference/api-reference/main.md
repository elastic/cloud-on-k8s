---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-reference.html
navigation_title: current
applies_to:
  deployment:
    eck: preview
---
% Generated documentation. Please do not edit.

# {{eck}} API Reference for main [k8s-api-reference-main]

## Packages
* [agent.k8s.elastic.co/v1alpha1](#agent-k8s-elastic-co-v1alpha1)
* [apm.k8s.elastic.co/v1](#apm-k8s-elastic-co-v1)
* [apm.k8s.elastic.co/v1beta1](#apm-k8s-elastic-co-v1beta1)
* [autoops.k8s.elastic.co/v1alpha1](#autoops-k8s-elastic-co-v1alpha1)
* [autoscaling.k8s.elastic.co/v1alpha1](#autoscaling-k8s-elastic-co-v1alpha1)
* [beat.k8s.elastic.co/v1beta1](#beat-k8s-elastic-co-v1beta1)
* [common.k8s.elastic.co/v1](#common-k8s-elastic-co-v1)
* [common.k8s.elastic.co/v1alpha1](#common-k8s-elastic-co-v1alpha1)
* [common.k8s.elastic.co/v1beta1](#common-k8s-elastic-co-v1beta1)
* [elasticsearch.k8s.elastic.co/v1](#elasticsearch-k8s-elastic-co-v1)
* [elasticsearch.k8s.elastic.co/v1beta1](#elasticsearch-k8s-elastic-co-v1beta1)
* [enterprisesearch.k8s.elastic.co/v1](#enterprisesearch-k8s-elastic-co-v1)
* [enterprisesearch.k8s.elastic.co/v1beta1](#enterprisesearch-k8s-elastic-co-v1beta1)
* [kibana.k8s.elastic.co/v1](#kibana-k8s-elastic-co-v1)
* [kibana.k8s.elastic.co/v1beta1](#kibana-k8s-elastic-co-v1beta1)
* [logstash.k8s.elastic.co/v1alpha1](#logstash-k8s-elastic-co-v1alpha1)
* [maps.k8s.elastic.co/v1alpha1](#maps-k8s-elastic-co-v1alpha1)
* [packageregistry.k8s.elastic.co/v1alpha1](#packageregistry-k8s-elastic-co-v1alpha1)
* [stackconfigpolicy.k8s.elastic.co/v1alpha1](#stackconfigpolicy-k8s-elastic-co-v1alpha1)


## agent.k8s.elastic.co/v1alpha1 [#agent-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API Schema definitions for the agent v1alpha1 API group

### Resource Types
- [Agent](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agent)



### Agent  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agent]

Agent is the Schema for the Agents API.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `agent.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `Agent` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)__ |  |


### AgentMode (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentmode]



:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)

:::



### AgentSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec]

AgentSpec defines the desired state of the Agent

:::{admonition} Appears In:
* [Agent](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agent)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the Agent. |
| *`elasticsearchRefs`* __[Output](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-output) array__ | ElasticsearchRefs is a reference to a list of Elasticsearch clusters running in the same Kubernetes cluster.<br>Due to existing limitations, only a single ES cluster is currently supported. |
| *`image`* __string__ | Image is the Agent Docker image to deploy. Version has to match the Agent in the image. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Agent configuration. At most one of [`Config`, `ConfigRef`] can be specified. |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Agent configuration.<br>Agent settings must be specified as yaml, under a single "agent.yml" entry. At most one of [`Config`, `ConfigRef`]<br>can be specified. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Agent.<br>Secrets data can be then referenced in the Agent config using the Secret's keys or as specified in `Entries` field of<br>each SecureSetting. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to an Elasticsearch resource in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |
| *`daemonSet`* __[DaemonSetSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-daemonsetspec)__ | DaemonSet specifies the Agent should be deployed as a DaemonSet, and allows providing its spec.<br>Cannot be used along with `deployment` or `statefulSet`. |
| *`deployment`* __[DeploymentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-deploymentspec)__ | Deployment specifies the Agent should be deployed as a Deployment, and allows providing its spec.<br>Cannot be used along with `daemonSet` or `statefulSet`. |
| *`statefulSet`* __[StatefulSetSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-statefulsetspec)__ | StatefulSet specifies the Agent should be deployed as a StatefulSet, and allows providing its spec.<br>Cannot be used along with `daemonSet` or `deployment`. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying DaemonSet or Deployment or StatefulSet. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for the Agent in Fleet mode with Fleet Server enabled. |
| *`mode`* __[AgentMode](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentmode)__ | Mode specifies the runtime mode for the Agent. The configuration can be specified locally through<br>`config` or `configRef` (`standalone` mode), or come from Fleet during runtime (`fleet` mode). Starting with<br>version 8.13.0 Fleet-managed agents support advanced configuration via a local configuration file.<br>See https://www.elastic.co/docs/reference/fleet/advanced-kubernetes-managed-by-fleet<br>Defaults to `standalone` mode. |
| *`fleetServerEnabled`* __boolean__ | FleetServerEnabled determines whether this Agent will launch Fleet Server. Don't set unless `mode` is set to `fleet`. |
| *`policyID`* __string__ | PolicyID determines into which Agent Policy this Agent will be enrolled.<br>This field will become mandatory in a future release, default policies are deprecated since 8.1.0. |
| *`kibanaRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | KibanaRef is a reference to Kibana where Fleet should be set up and this Agent should be enrolled. Don't set<br>unless `mode` is set to `fleet`. |
| *`fleetServerRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | FleetServerRef is a reference to Fleet Server that this Agent should connect to to obtain it's configuration.<br>Don't set unless `mode` is set to `fleet`.<br>References to Fleet servers running outside the Kubernetes cluster via the `secretName` attribute are not supported. |


### DaemonSetSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-daemonsetspec]



:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)

:::

| Field | Description |
| --- | --- |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ |  |
| *`updateStrategy`* __[DaemonSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#daemonsetupdatestrategy-v1-apps)__ |  |


### DeploymentSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-deploymentspec]



:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)

:::

| Field | Description |
| --- | --- |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ |  |
| *`replicas`* __integer__ |  |
| *`strategy`* __[DeploymentStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#deploymentstrategy-v1-apps)__ |  |


### Output  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-output]



:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)

:::

| Field | Description |
| --- | --- |
| *`ObjectSelector`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ |  |
| *`outputName`* __string__ |  |


### StatefulSetSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-statefulsetspec]



:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)

:::

| Field | Description |
| --- | --- |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ |  |
| *`replicas`* __integer__ |  |
| *`serviceName`* __string__ |  |
| *`podManagementPolicy`* __[PodManagementPolicyType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podmanagementpolicytype-v1-apps)__ | PodManagementPolicy controls how pods are created during initial scale up,<br>when replacing pods on nodes, or when scaling down. The default policy is<br>`Parallel`, where pods are created in parallel to match the desired scale<br>without waiting, and on scale down will delete all pods at once.<br>The alternative policy is `OrderedReady`, the default for vanilla kubernetes<br>StatefulSets, where pods are created in increasing order in increasing order<br>(pod-0, then pod-1, etc.) and the controller will wait until each pod is ready before<br>continuing. When scaling down, the pods are removed in the opposite order. |
| *`volumeClaimTemplates`* __[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array__ | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod.<br>Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.<br>Items defined here take precedence over any default claims added by the operator with the same name. |



## apm.k8s.elastic.co/v1 [#apm-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing APM Server resources.

### Resource Types
- [ApmServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserver)



### ApmServer  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserver]

ApmServer represents an APM Server resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `apm.k8s.elastic.co/v1` |
| *`kind`* __string__ | `ApmServer` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec)__ |  |


### ApmServerSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec]

ApmServerSpec holds the specification of an APM Server.

:::{admonition} Appears In:
* [ApmServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserver)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the APM Server. |
| *`image`* __string__ | Image is the APM Server Docker image to deploy. |
| *`count`* __integer__ | Count of APM Server instances to deploy. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the APM Server configuration. See: https://www.elastic.co/guide/en/apm/server/current/configuring-howto-apm-server.html |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for the APM Server resource. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to the output Elasticsearch cluster running in the same Kubernetes cluster. |
| *`kibanaRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster.<br>It allows APM agent central configuration management in Kibana. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the APM Server pods. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |



## apm.k8s.elastic.co/v1beta1 [#apm-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for managing APM Server resources.

### Resource Types
- [ApmServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserver)



### ApmServer  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserver]

ApmServer represents an APM Server resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `apm.k8s.elastic.co/v1beta1` |
| *`kind`* __string__ | `ApmServer` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec)__ |  |


### ApmServerSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec]

ApmServerSpec holds the specification of an APM Server.

:::{admonition} Appears In:
* [ApmServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserver)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the APM Server. |
| *`image`* __string__ | Image is the APM Server Docker image to deploy. |
| *`count`* __integer__ | Count of APM Server instances to deploy. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-config)__ | Config holds the APM Server configuration. See: https://www.elastic.co/guide/en/apm/server/current/configuring-howto-apm-server.html |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig)__ | HTTP holds the HTTP layer configuration for the APM Server resource. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-objectselector)__ | ElasticsearchRef is a reference to the output Elasticsearch cluster running in the same Kubernetes cluster. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the APM Server pods. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for APM Server. |



## autoops.k8s.elastic.co/v1alpha1 [#autoops-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing AutoOpsAgentPolicy resources.

### Resource Types
- [AutoOpsAgentPolicy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicy)



### AutoOpsAgentPolicy  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicy]

AutoOpsAgentPolicy represents an Elastic AutoOps Policy resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `autoops.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `AutoOpsAgentPolicy` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[AutoOpsAgentPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicyspec)__ |  |


### AutoOpsAgentPolicySpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicyspec]



:::{admonition} Appears In:
* [AutoOpsAgentPolicy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicy)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the AutoOpsAgentPolicy. |
| *`resourceSelector`* __[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta)__ | ResourceSelector is a label selector for the resources to be configured.<br>Any Elasticsearch instances that match the selector will be configured to send data to AutoOps. |
| *`namespaceSelector`* __[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta)__ | NamespaceSelector is a namespace selector for the resources to be configured.<br>Any Elasticsearch instances that belong to the selected namespaces will be configured to send data to AutoOps. |
| *`autoOpsRef`* __[AutoOpsRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsref)__ | AutoOpsRef defines a reference to a secret containing connection details for AutoOps via Cloud Connect. |
| *`image`* __string__ | Image is the AutoOps Agent Docker image to deploy. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Agent pods |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access to Elasticsearch resources in different namespaces.<br>Can only be used if ECK is enforcing RBAC on references (--enforce-rbac-on-refs flag).<br>The service account must have "get" permission on elasticsearch.k8s.elastic.co/elasticsearches<br>in the target namespaces. |


### AutoOpsRef  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsref]

AutoOpsRef defines a reference to a secret containing connection details for AutoOps via Cloud Connect.

:::{admonition} Appears In:
* [AutoOpsAgentPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoops-v1alpha1-autoopsagentpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName references a Secret containing connection details for external AutoOps.<br>Required when connecting via Cloud Connect. The secret must contain:<br>- `cloud-connected-mode-api-key`: Cloud Connected Mode API key<br>- `autoops-otel-url`: AutoOps OpenTelemetry endpoint URL<br>- `autoops-token`: AutoOps authentication token<br>- `cloud-connected-mode-api-url`: (optional) Cloud Connected Mode API URL<br>This field cannot be used in combination with `name`. |





## autoscaling.k8s.elastic.co/v1alpha1 [#autoscaling-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing ElasticsearchAutoscaler resources.

### Resource Types
- [ElasticsearchAutoscaler](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler)



### ElasticsearchAutoscaler  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler]

ElasticsearchAutoscaler represents an ElasticsearchAutoscaler resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `autoscaling.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `ElasticsearchAutoscaler` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ElasticsearchAutoscalerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec)__ |  |


### ElasticsearchAutoscalerSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec]

ElasticsearchAutoscalerSpec holds the specification of an Elasticsearch autoscaler resource.

:::{admonition} Appears In:
* [ElasticsearchAutoscaler](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler)

:::

| Field | Description |
| --- | --- |
| *`elasticsearchRef`* __[ElasticsearchRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchref)__ |  |
| *`pollingPeriod`* __[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)__ | PollingPeriod is the period at which to synchronize with the Elasticsearch autoscaling API. |


### ElasticsearchRef  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchref]

ElasticsearchRef is a reference to an Elasticsearch cluster that exists in the same namespace.

:::{admonition} Appears In:
* [ElasticsearchAutoscalerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name is the name of the Elasticsearch resource to scale automatically. |



## beat.k8s.elastic.co/v1beta1 [#beat-k8s-elastic-co-v1beta1]

Package v1beta1 contains API Schema definitions for the beat v1beta1 API group

### Resource Types
- [Beat](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beat)



### Beat  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beat]

Beat is the Schema for the Beats API.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `beat.k8s.elastic.co/v1beta1` |
| *`kind`* __string__ | `Beat` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)__ |  |


### BeatSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec]

BeatSpec defines the desired state of a Beat.

:::{admonition} Appears In:
* [Beat](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beat)

:::

| Field | Description |
| --- | --- |
| *`type`* __string__ | Type is the type of the Beat to deploy (filebeat, metricbeat, heartbeat, auditbeat, journalbeat, packetbeat, and so on).<br>Any string can be used, but well-known types will have the image field defaulted and have the appropriate<br>Elasticsearch roles created automatically. It also allows for dashboard setup when combined with a `KibanaRef`. |
| *`version`* __string__ | Version of the Beat. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster. |
| *`kibanaRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | KibanaRef is a reference to a Kibana instance running in the same Kubernetes cluster.<br>It allows automatic setup of dashboards and visualizations. |
| *`image`* __string__ | Image is the Beat Docker image to deploy. Version and Type have to match the Beat in the image. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Beat configuration. At most one of [`Config`, `ConfigRef`] can be specified. |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Beat configuration.<br>Beat settings must be specified as yaml, under a single "beat.yml" entry. At most one of [`Config`, `ConfigRef`]<br>can be specified. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Beat.<br>Secrets data can be then referenced in the Beat config using the Secret's keys or as specified in `Entries` field of<br>each SecureSetting. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |
| *`daemonSet`* __[DaemonSetSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-daemonsetspec)__ | DaemonSet specifies the Beat should be deployed as a DaemonSet, and allows providing its spec.<br>Cannot be used along with `deployment`. If both are absent a default for the Type is used. |
| *`deployment`* __[DeploymentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-deploymentspec)__ | Deployment specifies the Beat should be deployed as a Deployment, and allows providing its spec.<br>Cannot be used along with `daemonSet`. If both are absent a default for the Type is used. |
| *`monitoring`* __[Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)__ | Monitoring enables you to collect and ship logs and metrics for this Beat.<br>Metricbeat and/or Filebeat sidecars are configured and send monitoring data to an<br>Elasticsearch monitoring cluster running in the same Kubernetes cluster. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying DaemonSet or Deployment. |


### DaemonSetSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-daemonsetspec]



:::{admonition} Appears In:
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)

:::

| Field | Description |
| --- | --- |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ |  |
| *`updateStrategy`* __[DaemonSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#daemonsetupdatestrategy-v1-apps)__ |  |


### DeploymentSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-deploymentspec]



:::{admonition} Appears In:
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)

:::

| Field | Description |
| --- | --- |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ |  |
| *`replicas`* __integer__ |  |
| *`strategy`* __[DeploymentStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#deploymentstrategy-v1-apps)__ |  |



## common.k8s.elastic.co/v1 [#common-k8s-elastic-co-v1]

Package v1 contains API schema definitions for common types used by all resources.





### Config  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config]

Config represents untyped YAML configuration.

:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [IndexTemplates](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates)
* [KibanaConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)
* [MapsSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec)
* [NodeSet](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-nodeset)
* [PackageRegistrySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistryspec)
* [Search](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-search)

:::



### ConfigMapRef  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configmapref]

ConfigMapRef is a reference to a config map that exists in the same namespace as the referring resource.

:::{admonition} Appears In:
* [TransportTLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions)

:::

| Field | Description |
| --- | --- |
| *`configMapName`* __string__ |  |


### ConfigSource  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource]

ConfigSource references configuration settings.

:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)
* [MapsSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec)
* [PackageRegistrySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistryspec)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |




### HTTPConfig  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig]

HTTPConfig holds the HTTP layer configuration for resources.

:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec)
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)
* [MapsSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec)
* [PackageRegistrySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistryspec)

:::

| Field | Description |
| --- | --- |
| *`service`* __[ServiceTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-servicetemplate)__ | Service defines the template for the associated Kubernetes Service object. |
| *`tls`* __[TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-tlsoptions)__ | TLS defines options for configuring TLS for HTTP. |






### KeyToPath  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-keytopath]

KeyToPath defines how to map a key in a Secret object to a filesystem path.

:::{admonition} Appears In:
* [SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource)

:::

| Field | Description |
| --- | --- |
| *`key`* __string__ | Key is the key contained in the secret. |
| *`path`* __string__ | Path is the relative file path to map the key to.<br>Path must not be an absolute file path and must not contain any ".." components. |


### LocalObjectSelector  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-localobjectselector]

LocalObjectSelector defines a reference to a Kubernetes object corresponding to an Elastic resource managed by the operator

:::{admonition} Appears In:
* [RemoteCluster](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remotecluster)

:::

| Field | Description |
| --- | --- |
| *`namespace`* __string__ | Namespace of the Kubernetes object. If empty, defaults to the current namespace. |
| *`name`* __string__ | Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK. |
| *`serviceName`* __string__ | ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced<br>object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of<br>the referenced resource is used. |


### LogsMonitoring  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-logsmonitoring]

LogsMonitoring holds a list of Elasticsearch clusters which receive logs data from
associated resources.

:::{admonition} Appears In:
* [Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)

:::

| Field | Description |
| --- | --- |
| *`elasticsearchRefs`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector) array__ | ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.<br>Due to existing limitations, only a single Elasticsearch cluster is currently supported. |


### MetricsMonitoring  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-metricsmonitoring]

MetricsMonitoring holds a list of Elasticsearch clusters which receive monitoring data from
associated resources.

:::{admonition} Appears In:
* [Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)

:::

| Field | Description |
| --- | --- |
| *`elasticsearchRefs`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector) array__ | ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster.<br>Due to existing limitations, only a single Elasticsearch cluster is currently supported. |


### Monitoring  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring]

Monitoring holds references to both the metrics, and logs Elasticsearch clusters for
configuring stack monitoring.

:::{admonition} Appears In:
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)

:::

| Field | Description |
| --- | --- |
| *`metrics`* __[MetricsMonitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-metricsmonitoring)__ | Metrics holds references to Elasticsearch clusters which receive monitoring data from this resource. |
| *`logs`* __[LogsMonitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-logsmonitoring)__ | Logs holds references to Elasticsearch clusters which receive log data from an associated resource. |


### ObjectSelector  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector]

ObjectSelector defines a reference to a Kubernetes object which can be an Elastic resource managed by the operator
or a Secret describing an external Elastic resource not managed by the operator.

:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchCluster](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-elasticsearchcluster)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)
* [LogsMonitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-logsmonitoring)
* [MapsSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec)
* [MetricsMonitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-metricsmonitoring)
* [Output](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-output)

:::

| Field | Description |
| --- | --- |
| *`namespace`* __string__ | Namespace of the Kubernetes object. If empty, defaults to the current namespace. |
| *`name`* __string__ | Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK. |
| *`serviceName`* __string__ | ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced<br>object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of<br>the referenced resource is used. |
| *`secretName`* __string__ | SecretName is the name of an existing Kubernetes secret that contains connection information for associating an<br>Elastic resource not managed by the operator.<br>The referenced secret must contain the following:<br>- `url`: the URL to reach the Elastic resource<br>- `username`: the username of the user to be authenticated to the Elastic resource<br>- `password`: the password of the user to be authenticated to the Elastic resource<br>- `ca.crt`: the CA certificate in PEM format (optional)<br>- `api-key`: the key to authenticate against the Elastic resource instead of a username and password (supported only for `elasticsearchRefs` in AgentSpec and in BeatSpec)<br>This field cannot be used in combination with the other fields name, namespace or serviceName. |


### PodDisruptionBudgetTemplate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-poddisruptionbudgettemplate]

PodDisruptionBudgetTemplate defines the template for creating a PodDisruptionBudget.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#poddisruptionbudgetspec-v1-policy)__ | Spec is the specification of the PDB. |


### SecretRef  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretref]

SecretRef is a reference to a secret that exists in the same namespace.

:::{admonition} Appears In:
* [ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)
* [FileRealmSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-filerealmsource)
* [RoleSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-rolesource)
* [TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-tlsoptions)
* [TransportTLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |


### SecretSource  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource]

SecretSource defines a data source based on a Kubernetes Secret.

:::{admonition} Appears In:
* [AgentSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [KibanaConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)
* [StackConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |
| *`entries`* __[KeyToPath](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-keytopath) array__ | Entries define how to project each key-value pair in the secret to filesystem paths.<br>If not defined, all keys will be projected to similarly named paths in the filesystem.<br>If defined, only the specified keys will be projected to the corresponding paths. |


### SelfSignedCertificate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-selfsignedcertificate]

SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.

:::{admonition} Appears In:
* [TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-tlsoptions)

:::

| Field | Description |
| --- | --- |
| *`subjectAltNames`* __[SubjectAlternativeName](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-subjectalternativename) array__ | SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate. |
| *`disabled`* __boolean__ | Disabled indicates that the provisioning of the self-signed certificate should be disabled. |




### ServiceTemplate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-servicetemplate]

ServiceTemplate defines the template for a Kubernetes Service.

:::{admonition} Appears In:
* [HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)
* [LogstashService](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashservice)
* [RemoteClusterServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterserver)
* [TransportConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transportconfig)

:::

| Field | Description |
| --- | --- |
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ServiceSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#servicespec-v1-core)__ | Spec is the specification of the service. |


### SubjectAlternativeName  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-subjectalternativename]

SubjectAlternativeName represents a SAN entry in a x509 certificate.

:::{admonition} Appears In:
* [SelfSignedCertificate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-selfsignedcertificate)
* [TransportTLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions)

:::

| Field | Description |
| --- | --- |
| *`dns`* __string__ | DNS is the DNS name of the subject. |
| *`ip`* __string__ | IP is the IP address of the subject. |


### TLSOptions  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-tlsoptions]

TLSOptions holds TLS configuration options.

:::{admonition} Appears In:
* [HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)
* [LogstashService](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashservice)

:::

| Field | Description |
| --- | --- |
| *`selfSignedCertificate`* __[SelfSignedCertificate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-selfsignedcertificate)__ | SelfSignedCertificate allows configuring the self-signed certificate generated by the operator. |
| *`certificate`* __[SecretRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretref)__ | Certificate is a reference to a Kubernetes secret that contains the certificate and private key for enabling TLS.<br>The referenced secret should contain the following:<br><br>- `ca.crt`: The certificate authority (optional).<br>- `tls.crt`: The certificate (or a chain).<br>- `tls.key`: The private key to the first certificate in the certificate chain. |



## common.k8s.elastic.co/v1alpha1 [#common-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for common types used by all resources.



### Condition  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-condition]

Condition represents Elasticsearch resource's condition.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [Conditions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditions)

:::

| Field | Description |
| --- | --- |
| *`type`* __[ConditionType](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditiontype)__ |  |
| *`status`* __[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#conditionstatus-v1-core)__ |  |
| *`lastTransitionTime`* __[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)__ |  |
| *`message`* __string__ |  |


### ConditionType (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditiontype]

ConditionType defines the condition of an Elasticsearch resource.

:::{admonition} Appears In:
* [Condition](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-condition)

:::



### Conditions ([Condition](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-condition))  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditions]



:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus)

:::

| Field | Description |
| --- | --- |
| *`type`* __[ConditionType](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditiontype)__ |  |
| *`status`* __[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#conditionstatus-v1-core)__ |  |
| *`lastTransitionTime`* __[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)__ |  |
| *`message`* __string__ |  |

















## common.k8s.elastic.co/v1beta1 [#common-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for common types used by all resources.





### Config  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-config]

Config represents untyped YAML configuration.

:::{admonition} Appears In:
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec)
* [NodeSet](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-nodeset)

:::



### HTTPConfig  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig]

HTTPConfig holds the HTTP layer configuration for resources.

:::{admonition} Appears In:
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec)
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec)

:::

| Field | Description |
| --- | --- |
| *`service`* __[ServiceTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-servicetemplate)__ | Service defines the template for the associated Kubernetes Service object. |
| *`tls`* __[TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-tlsoptions)__ | TLS defines options for configuring TLS for HTTP. |


### KeyToPath  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-keytopath]

KeyToPath defines how to map a key in a Secret object to a filesystem path.

:::{admonition} Appears In:
* [SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretsource)

:::

| Field | Description |
| --- | --- |
| *`key`* __string__ | Key is the key contained in the secret. |
| *`path`* __string__ | Path is the relative file path to map the key to.<br>Path must not be an absolute file path and must not contain any ".." components. |


### ObjectSelector  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-objectselector]

ObjectSelector defines a reference to a Kubernetes object.

:::{admonition} Appears In:
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of the Kubernetes object. |
| *`namespace`* __string__ | Namespace of the Kubernetes object. If empty, defaults to the current namespace. |


### PodDisruptionBudgetTemplate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-poddisruptionbudgettemplate]

PodDisruptionBudgetTemplate defines the template for creating a PodDisruptionBudget.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#poddisruptionbudgetspec-v1beta1-policy)__ | Spec is the specification of the PDB. |


### SecretRef  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretref]

SecretRef is a reference to a secret that exists in the same namespace.

:::{admonition} Appears In:
* [TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-tlsoptions)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |


### SecretSource  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretsource]

SecretSource defines a data source based on a Kubernetes Secret.

:::{admonition} Appears In:
* [ApmServerSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-apm-v1beta1-apmserverspec)
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)
* [KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |
| *`entries`* __[KeyToPath](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-keytopath) array__ | Entries define how to project each key-value pair in the secret to filesystem paths.<br>If not defined, all keys will be projected to similarly named paths in the filesystem.<br>If defined, only the specified keys will be projected to the corresponding paths. |


### SelfSignedCertificate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-selfsignedcertificate]

SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.

:::{admonition} Appears In:
* [TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-tlsoptions)

:::

| Field | Description |
| --- | --- |
| *`subjectAltNames`* __[SubjectAlternativeName](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-subjectalternativename) array__ | SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate. |
| *`disabled`* __boolean__ | Disabled indicates that the provisioning of the self-signed certifcate should be disabled. |


### ServiceTemplate  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-servicetemplate]

ServiceTemplate defines the template for a Kubernetes Service.

:::{admonition} Appears In:
* [HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig)

:::

| Field | Description |
| --- | --- |
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ServiceSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#servicespec-v1-core)__ | Spec is the specification of the service. |


### SubjectAlternativeName  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-subjectalternativename]

SubjectAlternativeName represents a SAN entry in a x509 certificate.

:::{admonition} Appears In:
* [SelfSignedCertificate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-selfsignedcertificate)

:::

| Field | Description |
| --- | --- |
| *`dns`* __string__ | DNS is the DNS name of the subject. |
| *`ip`* __string__ | IP is the IP address of the subject. |


### TLSOptions  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-tlsoptions]

TLSOptions holds TLS configuration options.

:::{admonition} Appears In:
* [HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig)

:::

| Field | Description |
| --- | --- |
| *`selfSignedCertificate`* __[SelfSignedCertificate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-selfsignedcertificate)__ | SelfSignedCertificate allows configuring the self-signed certificate generated by the operator. |
| *`certificate`* __[SecretRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretref)__ | Certificate is a reference to a Kubernetes secret that contains the certificate and private key for enabling TLS.<br>The referenced secret should contain the following:<br><br>- `ca.crt`: The certificate authority (optional).<br>- `tls.crt`: The certificate (or a chain).<br>- `tls.key`: The private key to the first certificate in the certificate chain. |



## elasticsearch.k8s.elastic.co/v1 [#elasticsearch-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing Elasticsearch resources.

### Resource Types
- [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearch)



### Auth  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-auth]

Auth contains user authentication and authorization security settings for Elasticsearch.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`roles`* __[RoleSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-rolesource) array__ | Roles to propagate to the Elasticsearch cluster. |
| *`fileRealm`* __[FileRealmSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-filerealmsource) array__ | FileRealm to propagate to the Elasticsearch cluster. |
| *`disableElasticUser`* __boolean__ | DisableElasticUser disables the default elastic user that is created by ECK. |




### ChangeBudget  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-changebudget]

ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.

:::{admonition} Appears In:
* [UpdateStrategy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-updatestrategy)

:::

| Field | Description |
| --- | --- |
| *`maxUnavailable`* __integer__ | MaxUnavailable is the maximum number of Pods that can be unavailable (not ready) during the update due to<br>circumstances under the control of the operator. Setting a negative value will disable this restriction.<br>Defaults to 1 if not specified. |
| *`maxSurge`* __integer__ | MaxSurge is the maximum number of new Pods that can be created exceeding the original number of Pods defined in<br>the specification. MaxSurge is only taken into consideration when scaling up. Setting a negative value will<br>disable the restriction. Defaults to unbounded if not specified. |




### DownscaleOperation  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-downscaleoperation]

DownscaleOperation provides details about in progress downscale operations.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [InProgressOperations](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-inprogressoperations)

:::

| Field | Description |
| --- | --- |
| *`lastUpdatedTime`* __[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)__ |  |
| *`nodes`* __[DownscaledNode](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-downscalednode) array__ | Nodes which are scheduled to be removed from the cluster. |
| *`stalled`* __boolean__ | Stalled represents a state where no progress can be made.<br>It is only available for clusters managed with the Elasticsearch shutdown API. |


### DownscaledNode  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-downscalednode]

DownscaledNode provides an overview of in progress changes applied by the operator to remove Elasticsearch nodes from the cluster.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [DownscaleOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-downscaleoperation)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of the Elasticsearch node that should be removed. |
| *`shutdownStatus`* __string__ | Shutdown status as returned by the Elasticsearch shutdown API.<br>If the Elasticsearch shutdown API is not available, the shutdown status is then inferred from the remaining<br>shards on the nodes, as observed by the operator. |
| *`explanation`* __string__ | Explanation provides details about an in progress node shutdown. It is only available for clusters managed with the<br>Elasticsearch shutdown API. |


### Elasticsearch  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearch]

Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `elasticsearch.k8s.elastic.co/v1` |
| *`kind`* __string__ | `Elasticsearch` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)__ |  |
| *`status`* __[ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus)__ |  |


### ElasticsearchHealth (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchhealth]

ElasticsearchHealth is the health of the cluster as returned by the health API.

:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus)

:::



### ElasticsearchOrchestrationPhase (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchorchestrationphase]

ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.

:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus)

:::



### ElasticsearchSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec]

ElasticsearchSpec holds the specification of an Elasticsearch cluster.

:::{admonition} Appears In:
* [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearch)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Elasticsearch. |
| *`image`* __string__ | Image is the Elasticsearch Docker image to deploy. |
| *`remoteClusterServer`* __[RemoteClusterServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterserver)__ | RemoteClusterServer specifies if the remote cluster server should be enabled.<br>This must be enabled if this cluster is a remote cluster which is expected to be accessed using API key authentication. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds HTTP layer settings for Elasticsearch. |
| *`transport`* __[TransportConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transportconfig)__ | Transport holds transport layer settings for Elasticsearch. |
| *`nodeSets`* __[NodeSet](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-nodeset) array__ | NodeSets allow specifying groups of Elasticsearch nodes sharing the same configuration and Pod templates. |
| *`updateStrategy`* __[UpdateStrategy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-updatestrategy)__ | UpdateStrategy specifies how updates to the cluster should be performed. |
| *`podDisruptionBudget`* __[PodDisruptionBudgetTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-poddisruptionbudgettemplate)__ | PodDisruptionBudget provides access to the default Pod disruption budget(s) for the Elasticsearch cluster.<br>The behavior depends on the license level.<br>With a Basic license or if podDisruptionBudget.spec is not empty:<br>  The default budget doesn't allow any Pod to be removed in case the cluster is not green or if there is only one node of type `data` or `master`.<br>  In all other cases the default podDisruptionBudget sets `minAvailable` equal to the total number of nodes minus 1.<br>With an Enterprise license and if podDisruptionBudget.spec is empty:<br>  The default budget is split into multiple budgets, each targeting a specific node role type allowing additional disruptions<br>  for certain roles according to the health status of the cluster.<br>    Example:<br>      All data roles (excluding frozen): allows disruptions only when the cluster is green.<br>      All other roles: allows disruptions only when the cluster is yellow or green.<br>To disable, set `podDisruptionBudget` to the empty value (`{}` in YAML). |
| *`auth`* __[Auth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-auth)__ | Auth contains user authentication and authorization security settings for Elasticsearch. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Elasticsearch. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. a remote Elasticsearch cluster) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |
| *`remoteClusters`* __[RemoteCluster](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remotecluster) array__ | RemoteClusters enables you to establish uni-directional connections to a remote Elasticsearch cluster. |
| *`volumeClaimDeletePolicy`* __[VolumeClaimDeletePolicy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-volumeclaimdeletepolicy)__ | VolumeClaimDeletePolicy sets the policy for handling deletion of PersistentVolumeClaims for all NodeSets.<br>Possible values are DeleteOnScaledownOnly and DeleteOnScaledownAndClusterDeletion. Defaults to DeleteOnScaledownAndClusterDeletion. |
| *`monitoring`* __[Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)__ | Monitoring enables you to collect and ship log and monitoring data of this Elasticsearch cluster.<br>See https://www.elastic.co/guide/en/elasticsearch/reference/current/monitor-elasticsearch-cluster.html.<br>Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different<br>Elasticsearch monitoring clusters running in the same Kubernetes cluster. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSets. |


### ElasticsearchStatus  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus]

ElasticsearchStatus represents the observed state of Elasticsearch.

:::{admonition} Appears In:
* [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearch)

:::

| Field | Description |
| --- | --- |
| *`availableNodes`* __integer__ | AvailableNodes is the number of available instances. |
| *`version`* __string__ | Version of the stack resource currently running. During version upgrades, multiple versions may run<br>in parallel: this value specifies the lowest version currently running. |
| *`health`* __[ElasticsearchHealth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchhealth)__ |  |
| *`phase`* __[ElasticsearchOrchestrationPhase](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchorchestrationphase)__ |  |
| *`conditions`* __[Conditions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1alpha1-conditions)__ | Conditions holds the current service state of an Elasticsearch cluster.<br>**This API is in technical preview and may be changed or removed in a future release.** |
| *`inProgressOperations`* __[InProgressOperations](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-inprogressoperations)__ | InProgressOperations represents changes being applied by the operator to the Elasticsearch cluster.<br>**This API is in technical preview and may be changed or removed in a future release.** |
| *`observedGeneration`* __integer__ | ObservedGeneration is the most recent generation observed for this Elasticsearch cluster.<br>It corresponds to the metadata generation, which is updated on mutation by the API Server.<br>If the generation observed in status diverges from the generation in metadata, the Elasticsearch<br>controller has not yet processed the changes contained in the Elasticsearch specification. |


### FieldSecurity  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-fieldsecurity]



:::{admonition} Appears In:
* [Search](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-search)

:::

| Field | Description |
| --- | --- |
| *`grant`* __string array__ |  |
| *`except`* __string array__ |  |


### FileRealmSource  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-filerealmsource]

FileRealmSource references users to create in the Elasticsearch cluster.

:::{admonition} Appears In:
* [Auth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-auth)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |


### InProgressOperations  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-inprogressoperations]

InProgressOperations provides details about in progress changes applied by the operator on the Elasticsearch cluster.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchstatus)

:::

| Field | Description |
| --- | --- |
| *`downscale`* __[DownscaleOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-downscaleoperation)__ |  |
| *`upgrade`* __[UpgradeOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upgradeoperation)__ |  |
| *`upscale`* __[UpscaleOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upscaleoperation)__ |  |


### NewNode  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-newnode]



:::{admonition} Appears In:
* [UpscaleOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upscaleoperation)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of the Elasticsearch node that should be added to the cluster. |
| *`status`* __[NewNodeStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-newnodestatus)__ | NewNodeStatus states if a new node is being created, or if the upscale is delayed. |
| *`message`* __string__ | Optional message to explain why a node may not be immediately added. |


### NewNodeStatus (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-newnodestatus]

NewNodeStatus provides details about the status of nodes which are expected to be created and added to the Elasticsearch cluster.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [NewNode](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-newnode)

:::







### NodeSet  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-nodeset]

NodeSet is the specification for a group of Elasticsearch nodes sharing the same configuration and a Pod template.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of this set of nodes. Becomes a part of the Elasticsearch node.name setting. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Elasticsearch configuration. |
| *`count`* __integer__ | Count of Elasticsearch nodes to deploy.<br>If the node set is managed by an autoscaling policy the initial value is automatically set by the autoscaling controller. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet. |
| *`volumeClaimTemplates`* __[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array__ | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet.<br>Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.<br>Items defined here take precedence over any default claims added by the operator with the same name. |


### RemoteCluster  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remotecluster]

RemoteCluster declares a remote Elasticsearch cluster connection.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name is the name of the remote cluster as it is set in the Elasticsearch settings.<br>The name is expected to be unique for each remote clusters. |
| *`elasticsearchRef`* __[LocalObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-localobjectselector)__ | ElasticsearchRef is a reference to an Elasticsearch cluster running within the same k8s cluster. |
| *`apiKey`* __[RemoteClusterAPIKey](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterapikey)__ | APIKey can be used to enable remote cluster access using Cross-Cluster API keys: https://www.elastic.co/guide/en/elasticsearch/reference/current/security-api-create-cross-cluster-api-key.html |


### RemoteClusterAPIKey  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterapikey]

RemoteClusterAPIKey defines a remote cluster API Key.

:::{admonition} Appears In:
* [RemoteCluster](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remotecluster)

:::

| Field | Description |
| --- | --- |
| *`access`* __[RemoteClusterAccess](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusteraccess)__ | Access is the name of the API Key. It is automatically generated if not set or empty. |


### RemoteClusterAccess  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusteraccess]

RemoteClusterAccess models the API key specification as documented in https://www.elastic.co/guide/en/elasticsearch/reference/current/security-api-create-cross-cluster-api-key.html

:::{admonition} Appears In:
* [RemoteClusterAPIKey](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterapikey)

:::

| Field | Description |
| --- | --- |
| *`search`* __[Search](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-search)__ |  |
| *`replication`* __[Replication](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-replication)__ |  |


### RemoteClusterServer  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusterserver]



:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`enabled`* __boolean__ |  |
| *`service`* __[ServiceTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-servicetemplate)__ | Service defines the template for the remote cluster server Service object. |


### Replication  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-replication]



:::{admonition} Appears In:
* [RemoteClusterAccess](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusteraccess)

:::

| Field | Description |
| --- | --- |
| *`names`* __string array__ |  |


### RoleSource  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-rolesource]

RoleSource references roles to create in the Elasticsearch cluster.

:::{admonition} Appears In:
* [Auth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-auth)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName is the name of the secret. |


### Search  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-search]



:::{admonition} Appears In:
* [RemoteClusterAccess](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-remoteclusteraccess)

:::

| Field | Description |
| --- | --- |
| *`names`* __string array__ |  |
| *`field_security`* __[FieldSecurity](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-fieldsecurity)__ |  |
| *`query`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ |  |
| *`allow_restricted_indices`* __boolean__ |  |


### SelfSignedTransportCertificates  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-selfsignedtransportcertificates]

SelfSignedTransportCertificates holds configuration for the self-signed certificates generated by the operator.

:::{admonition} Appears In:
* [TransportTLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions)

:::

| Field | Description |
| --- | --- |
| *`disabled`* __boolean__ | Disabled indicates that provisioning of the self-signed certificates should be disabled. |


### TransportConfig  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transportconfig]

TransportConfig holds the transport layer settings for Elasticsearch.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`service`* __[ServiceTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-servicetemplate)__ | Service defines the template for the associated Kubernetes Service object. |
| *`tls`* __[TransportTLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions)__ | TLS defines options for configuring TLS on the transport layer. |


### TransportTLSOptions  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transporttlsoptions]



:::{admonition} Appears In:
* [TransportConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-transportconfig)

:::

| Field | Description |
| --- | --- |
| *`otherNameSuffix`* __string__ | OtherNameSuffix when defined will be prefixed with the Pod name and used as the common name,<br>and the first DNSName, as well as an OtherName required by Elasticsearch in the Subject Alternative Name<br>extension of each Elasticsearch node's transport TLS certificate.<br>Example: if set to "node.cluster.local", the generated certificate will have its otherName set to "<pod_name>.node.cluster.local". |
| *`subjectAltNames`* __[SubjectAlternativeName](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-subjectalternativename) array__ | SubjectAlternativeNames is a list of SANs to include in the generated node transport TLS certificates. |
| *`certificate`* __[SecretRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretref)__ | Certificate is a reference to a Kubernetes secret that contains the CA certificate<br>and private key for generating node certificates.<br>The referenced secret should contain the following:<br><br>- `ca.crt`: The CA certificate in PEM format.<br>- `ca.key`: The private key for the CA certificate in PEM format. |
| *`certificateAuthorities`* __[ConfigMapRef](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configmapref)__ | CertificateAuthorities is a reference to a config map that contains one or more x509 certificates for<br>trusted authorities in PEM format. The certificates need to be in a file called `ca.crt`. |
| *`selfSignedCertificates`* __[SelfSignedTransportCertificates](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-selfsignedtransportcertificates)__ | SelfSignedCertificates allows configuring the self-signed certificate generated by the operator. |


### UpdateStrategy  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-updatestrategy]

UpdateStrategy specifies how updates to the cluster should be performed.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`changeBudget`* __[ChangeBudget](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-changebudget)__ | ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster. |


### UpgradeOperation  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upgradeoperation]

UpgradeOperation provides an overview of the pending or in progress changes applied by the operator to update the Elasticsearch nodes in the cluster.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [InProgressOperations](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-inprogressoperations)

:::

| Field | Description |
| --- | --- |
| *`lastUpdatedTime`* __[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)__ |  |
| *`nodes`* __[UpgradedNode](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upgradednode) array__ | Nodes that must be restarted for upgrade. |


### UpgradedNode  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upgradednode]

UpgradedNode provides details about the status of nodes which are expected to be updated.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [UpgradeOperation](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upgradeoperation)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of the Elasticsearch node that should be upgraded. |
| *`status`* __string__ | Status states if the node is either in the process of being deleted for an upgrade,<br>or blocked by a predicate or another condition stated in the message field. |
| *`message`* __string__ | Optional message to explain why a node may not be immediately restarted for upgrade. |
| *`predicate`* __string__ | Predicate is the name of the predicate currently preventing this node from being deleted for an upgrade. |


### UpscaleOperation  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-upscaleoperation]

UpscaleOperation provides an overview of in progress changes applied by the operator to add Elasticsearch nodes to the cluster.
**This API is in technical preview and may be changed or removed in a future release.**

:::{admonition} Appears In:
* [InProgressOperations](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-inprogressoperations)

:::

| Field | Description |
| --- | --- |
| *`lastUpdatedTime`* __[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)__ |  |
| *`nodes`* __[NewNode](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-newnode) array__ | Nodes expected to be added by the operator. |


### VolumeClaimDeletePolicy (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-volumeclaimdeletepolicy]

VolumeClaimDeletePolicy describes the delete policy for handling PersistentVolumeClaims that hold Elasticsearch data.
Inspired by https://github.com/kubernetes/enhancements/pull/2440

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1-elasticsearchspec)

:::




## elasticsearch.k8s.elastic.co/v1beta1 [#elasticsearch-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for managing Elasticsearch resources.

### Resource Types
- [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearch)



### ChangeBudget  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-changebudget]

ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster.

:::{admonition} Appears In:
* [UpdateStrategy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-updatestrategy)

:::

| Field | Description |
| --- | --- |
| *`maxUnavailable`* __integer__ | MaxUnavailable is the maximum number of pods that can be unavailable (not ready) during the update due to<br>circumstances under the control of the operator. Setting a negative value will disable this restriction.<br>Defaults to 1 if not specified. |
| *`maxSurge`* __integer__ | MaxSurge is the maximum number of new pods that can be created exceeding the original number of pods defined in<br>the specification. MaxSurge is only taken into consideration when scaling up. Setting a negative value will<br>disable the restriction. Defaults to unbounded if not specified. |




### Elasticsearch  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearch]

Elasticsearch represents an Elasticsearch resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `elasticsearch.k8s.elastic.co/v1beta1` |
| *`kind`* __string__ | `Elasticsearch` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)__ |  |
| *`status`* __[ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)__ |  |


### ElasticsearchHealth (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchhealth]

ElasticsearchHealth is the health of the cluster as returned by the health API.

:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)

:::



### ElasticsearchOrchestrationPhase (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchorchestrationphase]

ElasticsearchOrchestrationPhase is the phase Elasticsearch is in from the controller point of view.

:::{admonition} Appears In:
* [ElasticsearchStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus)

:::



### ElasticsearchSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec]

ElasticsearchSpec holds the specification of an Elasticsearch cluster.

:::{admonition} Appears In:
* [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearch)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Elasticsearch. |
| *`image`* __string__ | Image is the Elasticsearch Docker image to deploy. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig)__ | HTTP holds HTTP layer settings for Elasticsearch. |
| *`nodeSets`* __[NodeSet](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-nodeset) array__ | NodeSets allow specifying groups of Elasticsearch nodes sharing the same configuration and Pod templates. |
| *`updateStrategy`* __[UpdateStrategy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-updatestrategy)__ | UpdateStrategy specifies how updates to the cluster should be performed. |
| *`podDisruptionBudget`* __[PodDisruptionBudgetTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-poddisruptionbudgettemplate)__ | PodDisruptionBudget provides access to the default pod disruption budget for the Elasticsearch cluster.<br>The default budget selects all cluster pods and sets `maxUnavailable` to 1. To disable, set `PodDisruptionBudget`<br>to the empty value (`{}` in YAML). |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Elasticsearch. |


### ElasticsearchStatus  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchstatus]

ElasticsearchStatus defines the observed state of Elasticsearch

:::{admonition} Appears In:
* [Elasticsearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearch)

:::

| Field | Description |
| --- | --- |
| *`health`* __[ElasticsearchHealth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchhealth)__ |  |
| *`phase`* __[ElasticsearchOrchestrationPhase](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchorchestrationphase)__ |  |




### NodeSet  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-nodeset]

NodeSet is the specification for a group of Elasticsearch nodes sharing the same configuration and a Pod template.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ | Name of this set of nodes. Becomes a part of the Elasticsearch node.name setting. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-config)__ | Config holds the Elasticsearch configuration. |
| *`count`* __integer__ | Count of Elasticsearch nodes to deploy. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Pods belonging to this NodeSet. |
| *`volumeClaimTemplates`* __[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array__ | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod in this NodeSet.<br>Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.<br>Items defined here take precedence over any default claims added by the operator with the same name. |


### UpdateStrategy  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-updatestrategy]

UpdateStrategy specifies how updates to the cluster should be performed.

:::{admonition} Appears In:
* [ElasticsearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

:::

| Field | Description |
| --- | --- |
| *`changeBudget`* __[ChangeBudget](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-elasticsearch-v1beta1-changebudget)__ | ChangeBudget defines the constraints to consider when applying changes to the Elasticsearch cluster. |



## enterprisesearch.k8s.elastic.co/v1 [#enterprisesearch-k8s-elastic-co-v1]

Package v1beta1 contains API schema definitions for managing Enterprise Search resources.

### Resource Types
- [EnterpriseSearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearch)



### EnterpriseSearch  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearch]

EnterpriseSearch is a Kubernetes CRD to represent Enterprise Search.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `enterprisesearch.k8s.elastic.co/v1` |
| *`kind`* __string__ | `EnterpriseSearch` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec)__ |  |


### EnterpriseSearchSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearchspec]

EnterpriseSearchSpec holds the specification of an Enterprise Search resource.

:::{admonition} Appears In:
* [EnterpriseSearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1-enterprisesearch)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Enterprise Search. |
| *`image`* __string__ | Image is the Enterprise Search Docker image to deploy. |
| *`count`* __integer__ | Count of Enterprise Search instances to deploy. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Enterprise Search configuration. |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Enterprise Search configuration.<br>Configuration settings are merged and have precedence over settings specified in `config`. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for Enterprise Search resource. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to the Elasticsearch cluster running in the same Kubernetes cluster. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on)<br>for the Enterprise Search pods. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |



## enterprisesearch.k8s.elastic.co/v1beta1 [#enterprisesearch-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for managing Enterprise Search resources.

### Resource Types
- [EnterpriseSearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearch)



### EnterpriseSearch  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearch]

EnterpriseSearch is a Kubernetes CRD to represent Enterprise Search.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `enterprisesearch.k8s.elastic.co/v1beta1` |
| *`kind`* __string__ | `EnterpriseSearch` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[EnterpriseSearchSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)__ |  |


### EnterpriseSearchSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec]

EnterpriseSearchSpec holds the specification of an Enterprise Search resource.

:::{admonition} Appears In:
* [EnterpriseSearch](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-enterprisesearch-v1beta1-enterprisesearch)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Enterprise Search. |
| *`image`* __string__ | Image is the Enterprise Search Docker image to deploy. |
| *`count`* __integer__ | Count of Enterprise Search instances to deploy. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Enterprise Search configuration. |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Enterprise Search configuration.<br>Configuration settings are merged and have precedence over settings specified in `config`. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for Enterprise Search resource. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to the Elasticsearch cluster running in the same Kubernetes cluster. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on)<br>for the Enterprise Search pods. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |



## kibana.k8s.elastic.co/v1 [#kibana-k8s-elastic-co-v1]

Package v1 contains API schema definitions for managing Kibana resources.

### Resource Types
- [Kibana](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibana)



### Kibana  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibana]

Kibana represents a Kibana resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `kibana.k8s.elastic.co/v1` |
| *`kind`* __string__ | `Kibana` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec)__ |  |


### KibanaSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibanaspec]

KibanaSpec holds the specification of a Kibana instance.

:::{admonition} Appears In:
* [Kibana](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1-kibana)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Kibana. |
| *`image`* __string__ | Image is the Kibana Docker image to deploy. |
| *`count`* __integer__ | Count of Kibana instances to deploy. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster. |
| *`packageRegistryRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | PackageRegistryRef is a reference to an Elastic Package Registry running in the same Kubernetes cluster. |
| *`enterpriseSearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | EnterpriseSearchRef is a reference to an EnterpriseSearch running in the same Kubernetes cluster.<br>Kibana provides the default Enterprise Search UI starting version 7.14. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Kibana configuration. See: https://www.elastic.co/guide/en/kibana/current/settings.html |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for Kibana. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |
| *`monitoring`* __[Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)__ | Monitoring enables you to collect and ship log and monitoring data of this Kibana.<br>See https://www.elastic.co/guide/en/kibana/current/xpack-monitoring.html.<br>Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different<br>Elasticsearch monitoring clusters running in the same Kubernetes cluster. |



## kibana.k8s.elastic.co/v1beta1 [#kibana-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for managing Kibana resources.

### Resource Types
- [Kibana](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibana)



### Kibana  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibana]

Kibana represents a Kibana resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `kibana.k8s.elastic.co/v1beta1` |
| *`kind`* __string__ | `Kibana` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[KibanaSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec)__ |  |


### KibanaSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibanaspec]

KibanaSpec holds the specification of a Kibana instance.

:::{admonition} Appears In:
* [Kibana](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-kibana-v1beta1-kibana)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Kibana. |
| *`image`* __string__ | Image is the Kibana Docker image to deploy. |
| *`count`* __integer__ | Count of Kibana instances to deploy. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-objectselector)__ | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-config)__ | Config holds the Kibana configuration. See: https://www.elastic.co/guide/en/kibana/current/settings.html |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-httpconfig)__ | HTTP holds the HTTP layer configuration for Kibana. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Kibana pods |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1beta1-secretsource) array__ | SecureSettings is a list of references to Kubernetes secrets containing sensitive configuration options for Kibana. |



## logstash.k8s.elastic.co/v1alpha1 [#logstash-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API Schema definitions for the logstash v1alpha1 API group

### Resource Types
- [Logstash](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstash)



### ElasticsearchCluster  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-elasticsearchcluster]

ElasticsearchCluster is a named reference to an Elasticsearch cluster which can be used in a Logstash pipeline.

:::{admonition} Appears In:
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)

:::

| Field | Description |
| --- | --- |
| *`ObjectSelector`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ |  |
| *`clusterName`* __string__ | ClusterName is an alias for the cluster to be used to refer to the Elasticsearch cluster in Logstash<br>configuration files, and will be used to identify "named clusters" in Logstash |


### Logstash  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstash]

Logstash is the Schema for the logstashes API



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `logstash.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `Logstash` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)__ |  |
| *`status`* __[LogstashStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashstatus)__ |  |


### LogstashHealth (string)  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashhealth]



:::{admonition} Appears In:
* [LogstashStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashstatus)

:::



### LogstashService  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashservice]



:::{admonition} Appears In:
* [LogstashSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec)

:::

| Field | Description |
| --- | --- |
| *`name`* __string__ |  |
| *`service`* __[ServiceTemplate](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-servicetemplate)__ | Service defines the template for the associated Kubernetes Service object. |
| *`tls`* __[TLSOptions](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-tlsoptions)__ | TLS defines options for configuring TLS for HTTP. |


### LogstashSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashspec]

LogstashSpec defines the desired state of Logstash

:::{admonition} Appears In:
* [Logstash](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstash)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the Logstash. |
| *`count`* __integer__ |  |
| *`image`* __string__ | Image is the Logstash Docker image to deploy. Version and Type have to match the Logstash in the image. |
| *`elasticsearchRefs`* __[ElasticsearchCluster](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-elasticsearchcluster) array__ | ElasticsearchRefs are references to Elasticsearch clusters running in the same Kubernetes cluster. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the Logstash configuration. At most one of [`Config`, `ConfigRef`] can be specified. |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Logstash configuration.<br>Logstash settings must be specified as yaml, under a single "logstash.yml" entry. At most one of [`Config`, `ConfigRef`]<br>can be specified. |
| *`pipelines`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config) array__ | Pipelines holds the Logstash Pipelines. At most one of [`Pipelines`, `PipelinesRef`] can be specified. |
| *`pipelinesRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | PipelinesRef contains a reference to an existing Kubernetes Secret holding the Logstash Pipelines.<br>Logstash pipelines must be specified as yaml, under a single "pipelines.yml" entry. At most one of [`Pipelines`, `PipelinesRef`]<br>can be specified. |
| *`services`* __[LogstashService](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashservice) array__ | Services contains details of services that Logstash should expose - similar to the HTTP layer configuration for the<br>rest of the stack, but also applicable for more use cases than the metrics API, as logstash may need to<br>be opened up for other services: Beats, TCP, UDP, etc, inputs. |
| *`monitoring`* __[Monitoring](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-monitoring)__ | Monitoring enables you to collect and ship log and monitoring data of this Logstash.<br>Metricbeat and Filebeat are deployed in the same Pod as sidecars and each one sends data to one or two different<br>Elasticsearch monitoring clusters running in the same Kubernetes cluster. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options for the Logstash pods. |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying StatefulSet. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings is a list of references to Kubernetes Secrets containing sensitive configuration options for the Logstash.<br>Secrets data can be then referenced in the Logstash config using the Secret's keys or as specified in `Entries` field of<br>each SecureSetting. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to Elasticsearch resource in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |
| *`updateStrategy`* __[StatefulSetUpdateStrategy](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#statefulsetupdatestrategy-v1-apps)__ | UpdateStrategy is a StatefulSetUpdateStrategy. The default type is "RollingUpdate". |
| *`volumeClaimTemplates`* __[PersistentVolumeClaim](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#persistentvolumeclaim-v1-core) array__ | VolumeClaimTemplates is a list of persistent volume claims to be used by each Pod.<br>Every claim in this list must have a matching volumeMount in one of the containers defined in the PodTemplate.<br>Items defined here take precedence over any default claims added by the operator with the same name. |


### LogstashStatus  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashstatus]

LogstashStatus defines the observed state of Logstash

:::{admonition} Appears In:
* [Logstash](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstash)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of the stack resource currently running. During version upgrades, multiple versions may run<br>in parallel: this value specifies the lowest version currently running. |
| *`expectedNodes`* __integer__ |  |
| *`availableNodes`* __integer__ |  |
| *`health`* __[LogstashHealth](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-logstash-v1alpha1-logstashhealth)__ |  |
| *`observedGeneration`* __integer__ | ObservedGeneration is the most recent generation observed for this Logstash instance.<br>It corresponds to the metadata generation, which is updated on mutation by the API Server.<br>If the generation observed in status diverges from the generation in metadata, the Logstash<br>controller has not yet processed the changes contained in the Logstash specification. |
| *`selector`* __string__ |  |



## maps.k8s.elastic.co/v1alpha1 [#maps-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing Elastic Maps Server resources.

### Resource Types
- [ElasticMapsServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserver)
- [ElasticMapsServerList](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserverlist)



### ElasticMapsServer  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserver]

ElasticMapsServer represents an Elastic Map Server resource in a Kubernetes cluster.

:::{admonition} Appears In:
* [ElasticMapsServerList](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserverlist)

:::

| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `maps.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `ElasticMapsServer` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[MapsSpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec)__ |  |


### ElasticMapsServerList  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserverlist]

ElasticMapsServerList contains a list of ElasticMapsServer



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `maps.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `ElasticMapsServerList` | 
| *`metadata`* __[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`items`* __[ElasticMapsServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserver) array__ |  |


### MapsSpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-mapsspec]

MapsSpec holds the specification of an Elastic Maps Server instance.

:::{admonition} Appears In:
* [ElasticMapsServer](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-maps-v1alpha1-elasticmapsserver)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Elastic Maps Server. |
| *`image`* __string__ | Image is the Elastic Maps Server Docker image to deploy. |
| *`count`* __integer__ | Count of Elastic Maps Server instances to deploy. |
| *`elasticsearchRef`* __[ObjectSelector](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-objectselector)__ | ElasticsearchRef is a reference to an Elasticsearch cluster running in the same Kubernetes cluster. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the ElasticMapsServer configuration. See: https://www.elastic.co/guide/en/kibana/current/maps-connect-to-ems.html#elastic-maps-server-configuration |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Elastic Maps Server configuration.<br>Configuration settings are merged and have precedence over settings specified in `config`. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for Elastic Maps Server. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Elastic Maps Server pods |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |
| *`serviceAccountName`* __string__ | ServiceAccountName is used to check access from the current resource to a resource (for ex. Elasticsearch) in a different namespace.<br>Can only be used if ECK is enforcing RBAC on references. |



## packageregistry.k8s.elastic.co/v1alpha1 [#packageregistry-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing Elastic Package Registry resources.

### Resource Types
- [PackageRegistry](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistry)
- [PackageRegistryList](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistrylist)



### PackageRegistry  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistry]

PackageRegistry represents an Elastic Package Registry resource in a Kubernetes cluster.

:::{admonition} Appears In:
* [PackageRegistryList](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistrylist)

:::

| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `packageregistry.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `PackageRegistry` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[PackageRegistrySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistryspec)__ |  |
| *`status`* __[PackageRegistryStatus](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistrystatus)__ |  |


### PackageRegistryList  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistrylist]

PackageRegistryList contains a list of PackageRegistry



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `packageregistry.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `PackageRegistryList` | 
| *`metadata`* __[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#listmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`items`* __[PackageRegistry](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistry) array__ |  |


### PackageRegistrySpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistryspec]

PackageRegistrySpec holds the specification of an Elastic Package Registry instance.

:::{admonition} Appears In:
* [PackageRegistry](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistry)

:::

| Field | Description |
| --- | --- |
| *`version`* __string__ | Version of Elastic Package Registry. |
| *`image`* __string__ | Image is the Elastic Package Registry Docker image to deploy. |
| *`count`* __integer__ | Count of Elastic Package Registry instances to deploy. |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the PackageRegistry configuration. See: https://github.com/elastic/package-registry/blob/main/config.reference.yml |
| *`configRef`* __[ConfigSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-configsource)__ | ConfigRef contains a reference to an existing Kubernetes Secret holding the Elastic Package Registry configuration.<br>Configuration settings are merged and have precedence over settings specified in `config`. |
| *`http`* __[HTTPConfig](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-httpconfig)__ | HTTP holds the HTTP layer configuration for Elastic Package Registry. |
| *`podTemplate`* __[PodTemplateSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#podtemplatespec-v1-core)__ | PodTemplate provides customisation options (labels, annotations, affinity rules, resource requests, and so on) for the Elastic Package Registry pods |
| *`revisionHistoryLimit`* __integer__ | RevisionHistoryLimit is the number of revisions to retain to allow rollback in the underlying Deployment. |


### PackageRegistryStatus  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistrystatus]

PackageRegistryStatus defines the observed state of Elastic Package Registry

:::{admonition} Appears In:
* [PackageRegistry](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-packageregistry-v1alpha1-packageregistry)

:::

| Field | Description |
| --- | --- |
| *`observedGeneration`* __integer__ | ObservedGeneration is the most recent generation observed for this Elastic Package Registry.<br>It corresponds to the metadata generation, which is updated on mutation by the API Server.<br>If the generation observed in status diverges from the generation in metadata, the Elastic Package Registry<br>controller has not yet processed the changes contained in the Elastic Package Registry specification. |



## stackconfigpolicy.k8s.elastic.co/v1alpha1 [#stackconfigpolicy-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing StackConfigPolicy resources.

### Resource Types
- [StackConfigPolicy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy)



### ElasticsearchConfigPolicySpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec]



:::{admonition} Appears In:
* [StackConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`clusterSettings`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | ClusterSettings holds the Elasticsearch cluster settings (/_cluster/settings) |
| *`snapshotRepositories`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | SnapshotRepositories holds the Snapshot Repositories settings (/_snapshot) |
| *`snapshotLifecyclePolicies`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | SnapshotLifecyclePolicies holds the Snapshot Lifecycle Policies settings (/_slm/policy) |
| *`securityRoleMappings`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | SecurityRoleMappings holds the Role Mappings settings (/_security/role_mapping) |
| *`indexLifecyclePolicies`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | IndexLifecyclePolicies holds the Index Lifecycle policies settings (/_ilm/policy) |
| *`ingestPipelines`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | IngestPipelines holds the Ingest Pipelines settings (/_ingest/pipeline) |
| *`indexTemplates`* __[IndexTemplates](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates)__ | IndexTemplates holds the Index and Component Templates settings |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the settings that go into elasticsearch.yml. |
| *`secretMounts`* __[SecretMount](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-secretmount) array__ | SecretMounts are additional Secrets that need to be mounted into the Elasticsearch pods. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings are additional Secrets that contain data to be configured to Elasticsearch's keystore. |




### IndexTemplates  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates]



:::{admonition} Appears In:
* [ElasticsearchConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`componentTemplates`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | ComponentTemplates holds the Component Templates settings (/_component_template) |
| *`composableIndexTemplates`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | ComposableIndexTemplates holds the Index Templates settings (/_index_template) |


### KibanaConfigPolicySpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec]



:::{admonition} Appears In:
* [StackConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`config`* __[Config](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-config)__ | Config holds the settings that go into kibana.yml. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | SecureSettings are additional Secrets that contain data to be configured to Kibana's keystore. |








### SecretMount  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-secretmount]

SecretMount contains information about additional secrets to be mounted to the elasticsearch pods

:::{admonition} Appears In:
* [ElasticsearchConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)

:::

| Field | Description |
| --- | --- |
| *`secretName`* __string__ | SecretName denotes the name of the secret that needs to be mounted to the elasticsearch pod |
| *`mountPath`* __string__ | MountPath denotes the path to which the secret should be mounted to inside the elasticsearch pod |


### StackConfigPolicy  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy]

StackConfigPolicy represents a StackConfigPolicy resource in a Kubernetes cluster.



| Field | Description |
| --- | --- |
| *`apiVersion`* __string__ | `stackconfigpolicy.k8s.elastic.co/v1alpha1` |
| *`kind`* __string__ | `StackConfigPolicy` | 
| *`metadata`* __[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)__ | Refer to Kubernetes API documentation for fields of `metadata`. |
| *`spec`* __[StackConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)__ |  |


### StackConfigPolicySpec  [#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec]



:::{admonition} Appears In:
* [StackConfigPolicy](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicy)

:::

| Field | Description |
| --- | --- |
| *`resourceSelector`* __[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#labelselector-v1-meta)__ |  |
| *`weight`* __integer__ | Weight determines the priority of this policy when multiple policies target the same resource.<br>Higher weight values take precedence. Defaults to 0. |
| *`secureSettings`* __[SecretSource](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-common-v1-secretsource) array__ | Deprecated: SecureSettings only applies to Elasticsearch and is deprecated. It must be set per application instead. |
| *`elasticsearch`* __[ElasticsearchConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)__ |  |
| *`kibana`* __[KibanaConfigPolicySpec](#github-com-elastic-cloud-on-k8s-v3-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)__ |  |


