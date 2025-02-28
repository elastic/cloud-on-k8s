---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-common-k8s-elastic-co-v1.html
applies_to:
  deployment:
    eck: all
---

# common.k8s.elastic.co/v1 [k8s-api-common-k8s-elastic-co-v1]

Package v1 contains API schema definitions for common types used by all resources.

## Config [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-config]

Config represents untyped YAML configuration.

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [IndexTemplates](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-indextemplates)
* [KibanaConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)
* [MapsSpec](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec)
* [NodeSet](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-nodeset)
* [Search](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-search)

::::



## ConfigMapRef [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configmapref]

ConfigMapRef is a reference to a config map that exists in the same namespace as the referring resource.

::::{admonition} Appears In:
* [TransportTLSOptions](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions)

::::


| Field | Description |
| --- | --- |
| **`configMapName`** *string*<br> |  |


## ConfigSource [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource]

ConfigSource references configuration settings.

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)
* [MapsSpec](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |


## HTTPConfig [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig]

HTTPConfig holds the HTTP layer configuration for resources.

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec)
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)
* [MapsSpec](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec)

::::


| Field | Description |
| --- | --- |
| **`service`** *[ServiceTemplate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-servicetemplate)*<br> | Service defines the template for the associated Kubernetes Service object.<br> |
| **`tls`** *[TLSOptions](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-tlsoptions)*<br> | TLS defines options for configuring TLS for HTTP.<br> |


## KeyToPath [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-keytopath]

KeyToPath defines how to map a key in a Secret object to a filesystem path.

::::{admonition} Appears In:
* [SecretSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource)

::::


| Field | Description |
| --- | --- |
| **`key`** *string*<br> | Key is the key contained in the secret.<br> |
| **`path`** *string*<br> | Path is the relative file path to map the key to. Path must not be an absolute file path and must not contain any ".." components.<br> |


## LocalObjectSelector [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-localobjectselector]

LocalObjectSelector defines a reference to a Kubernetes object corresponding to an Elastic resource managed by the operator

::::{admonition} Appears In:
* [RemoteCluster](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-remotecluster)

::::


| Field | Description |
| --- | --- |
| **`namespace`** *string*<br> | Namespace of the Kubernetes object. If empty, defaults to the current namespace.<br> |
| **`name`** *string*<br> | Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK.<br> |
| **`serviceName`** *string*<br> | ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of the referenced resource is used.<br> |


## LogsMonitoring [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-logsmonitoring]

LogsMonitoring holds a list of Elasticsearch clusters which receive logs data from associated resources.

::::{admonition} Appears In:
* [Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)

::::


| Field | Description |
| --- | --- |
| **`elasticsearchRefs`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector) array*<br> | ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster. Due to existing limitations, only a single Elasticsearch cluster is currently supported.<br> |


## MetricsMonitoring [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-metricsmonitoring]

MetricsMonitoring holds a list of Elasticsearch clusters which receive monitoring data from associated resources.

::::{admonition} Appears In:
* [Monitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring)

::::


| Field | Description |
| --- | --- |
| **`elasticsearchRefs`** *[ObjectSelector](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector) array*<br> | ElasticsearchRefs is a reference to a list of monitoring Elasticsearch clusters running in the same Kubernetes cluster. Due to existing limitations, only a single Elasticsearch cluster is currently supported.<br> |


## Monitoring [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-monitoring]

Monitoring holds references to both the metrics, and logs Elasticsearch clusters for configuring stack monitoring.

::::{admonition} Appears In:
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)

::::


| Field | Description |
| --- | --- |
| **`metrics`** *[MetricsMonitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-metricsmonitoring)*<br> | Metrics holds references to Elasticsearch clusters which receive monitoring data from this resource.<br> |
| **`logs`** *[LogsMonitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-logsmonitoring)*<br> | Logs holds references to Elasticsearch clusters which receive log data from an associated resource.<br> |


## ObjectSelector [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-objectselector]

ObjectSelector defines a reference to a Kubernetes object which can be an Elastic resource managed by the operator or a Secret describing an external Elastic resource not managed by the operator.

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchCluster](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-elasticsearchcluster)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1-enterprisesearchspec)
* [EnterpriseSearchSpec](k8s-api-enterprisesearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-enterprisesearch-v1beta1-enterprisesearchspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)
* [LogsMonitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-logsmonitoring)
* [MapsSpec](k8s-api-maps-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-maps-v1alpha1-mapsspec)
* [MetricsMonitoring](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-metricsmonitoring)
* [Output](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-output)

::::


| Field | Description |
| --- | --- |
| **`namespace`** *string*<br> | Namespace of the Kubernetes object. If empty, defaults to the current namespace.<br> |
| **`name`** *string*<br> | Name of an existing Kubernetes object corresponding to an Elastic resource managed by ECK.<br> |
| **`serviceName`** *string*<br> | ServiceName is the name of an existing Kubernetes service which is used to make requests to the referenced object. It has to be in the same namespace as the referenced resource. If left empty, the default HTTP service of the referenced resource is used.<br> |
| **`secretName`** *string*<br> | SecretName is the name of an existing Kubernetes secret that contains connection information for associating an Elastic resource not managed by the operator. The referenced secret must contain the following: - `url`: the URL to reach the Elastic resource - `username`: the username of the user to be authenticated to the Elastic resource - `password`: the password of the user to be authenticated to the Elastic resource - `ca.crt`: the CA certificate in PEM format (optional) - `api-key`: the key to authenticate against the Elastic resource instead of a username and password (supported only for `elasticsearchRefs` in AgentSpec and in BeatSpec) This field cannot be used in combination with the other fields name, namespace or serviceName.<br> |


## PodDisruptionBudgetTemplate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-poddisruptionbudgettemplate]

PodDisruptionBudgetTemplate defines the template for creating a PodDisruptionBudget.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#poddisruptionbudgetspec-v1-policy)*<br> | Spec is the specification of the PDB.<br> |


## SecretRef [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretref]

SecretRef is a reference to a secret that exists in the same namespace.

::::{admonition} Appears In:
* [ConfigSource](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-configsource)
* [FileRealmSource](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-filerealmsource)
* [RoleSource](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-rolesource)
* [TLSOptions](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-tlsoptions)
* [TransportTLSOptions](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |


## SecretSource [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretsource]

SecretSource defines a data source based on a Kubernetes Secret.

::::{admonition} Appears In:
* [AgentSpec](k8s-api-agent-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-agent-v1alpha1-agentspec)
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1-apmserverspec)
* [BeatSpec](k8s-api-beat-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-beat-v1beta1-beatspec)
* [ElasticsearchConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-elasticsearchconfigpolicyspec)
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchspec)
* [KibanaConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-kibanaconfigpolicyspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1-kibanaspec)
* [LogstashSpec](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashspec)
* [StackConfigPolicySpec](k8s-api-stackconfigpolicy-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-stackconfigpolicy-v1alpha1-stackconfigpolicyspec)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |
| **`entries`** *[KeyToPath](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-keytopath) array*<br> | Entries define how to project each key-value pair in the secret to filesystem paths. If not defined, all keys will be projected to similarly named paths in the filesystem. If defined, only the specified keys will be projected to the corresponding paths.<br> |


## SelfSignedCertificate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-selfsignedcertificate]

SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.

::::{admonition} Appears In:
* [TLSOptions](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-tlsoptions)

::::


| Field | Description |
| --- | --- |
| **`subjectAltNames`** *[SubjectAlternativeName](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-subjectalternativename) array*<br> | SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate.<br> |
| **`disabled`** *boolean*<br> | Disabled indicates that the provisioning of the self-signed certifcate should be disabled.<br> |


## ServiceTemplate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-servicetemplate]

ServiceTemplate defines the template for a Kubernetes Service.

::::{admonition} Appears In:
* [HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)
* [LogstashService](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashservice)
* [TransportConfig](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transportconfig)

::::


| Field | Description |
| --- | --- |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ServiceSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#servicespec-v1-core)*<br> | Spec is the specification of the service.<br> |


## SubjectAlternativeName [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-subjectalternativename]

SubjectAlternativeName represents a SAN entry in a x509 certificate.

::::{admonition} Appears In:
* [SelfSignedCertificate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-selfsignedcertificate)
* [TransportTLSOptions](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-transporttlsoptions)

::::


| Field | Description |
| --- | --- |
| **`dns`** *string*<br> | DNS is the DNS name of the subject.<br> |
| **`ip`** *string*<br> | IP is the IP address of the subject.<br> |


## TLSOptions [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-tlsoptions]

TLSOptions holds TLS configuration options.

::::{admonition} Appears In:
* [HTTPConfig](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-httpconfig)
* [LogstashService](k8s-api-logstash-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-logstash-v1alpha1-logstashservice)

::::


| Field | Description |
| --- | --- |
| **`selfSignedCertificate`** *[SelfSignedCertificate](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-selfsignedcertificate)*<br> | SelfSignedCertificate allows configuring the self-signed certificate generated by the operator.<br> |
| **`certificate`** *[SecretRef](k8s-api-common-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1-secretref)*<br> | Certificate is a reference to a Kubernetes secret that contains the certificate and private key for enabling TLS. The referenced secret should contain the following:<br><br>* `ca.crt`: The certificate authority (optional).<br>* `tls.crt`: The certificate (or a chain).<br>* `tls.key`: The private key to the first certificate in the certificate chain.<br> |


