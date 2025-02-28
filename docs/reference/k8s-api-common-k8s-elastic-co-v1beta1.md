---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-common-k8s-elastic-co-v1beta1.html
applies_to:
  deployment:
    eck: all
---

# common.k8s.elastic.co/v1beta1 [k8s-api-common-k8s-elastic-co-v1beta1]

Package v1beta1 contains API schema definitions for common types used by all resources.

## Config [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-config]

Config represents untyped YAML configuration.

::::{admonition} Appears In:
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1beta1-apmserverspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1beta1-kibanaspec)
* [NodeSet](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-nodeset)

::::



## HTTPConfig [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-httpconfig]

HTTPConfig holds the HTTP layer configuration for resources.

::::{admonition} Appears In:
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1beta1-apmserverspec)
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1beta1-kibanaspec)

::::


| Field | Description |
| --- | --- |
| **`service`** *[ServiceTemplate](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-servicetemplate)*<br> | Service defines the template for the associated Kubernetes Service object.<br> |
| **`tls`** *[TLSOptions](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-tlsoptions)*<br> | TLS defines options for configuring TLS for HTTP.<br> |


## KeyToPath [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-keytopath]

KeyToPath defines how to map a key in a Secret object to a filesystem path.

::::{admonition} Appears In:
* [SecretSource](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-secretsource)

::::


| Field | Description |
| --- | --- |
| **`key`** *string*<br> | Key is the key contained in the secret.<br> |
| **`path`** *string*<br> | Path is the relative file path to map the key to. Path must not be an absolute file path and must not contain any ".." components.<br> |


## ObjectSelector [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-objectselector]

ObjectSelector defines a reference to a Kubernetes object.

::::{admonition} Appears In:
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1beta1-apmserverspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1beta1-kibanaspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name of the Kubernetes object.<br> |
| **`namespace`** *string*<br> | Namespace of the Kubernetes object. If empty, defaults to the current namespace.<br> |


## PodDisruptionBudgetTemplate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-poddisruptionbudgettemplate]

PodDisruptionBudgetTemplate defines the template for creating a PodDisruptionBudget.

::::{admonition} Appears In:
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)

::::


| Field | Description |
| --- | --- |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[PodDisruptionBudgetSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#poddisruptionbudgetspec-v1beta1-policy)*<br> | Spec is the specification of the PDB.<br> |


## SecretRef [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-secretref]

SecretRef is a reference to a secret that exists in the same namespace.

::::{admonition} Appears In:
* [TLSOptions](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-tlsoptions)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |


## SecretSource [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-secretsource]

SecretSource defines a data source based on a Kubernetes Secret.

::::{admonition} Appears In:
* [ApmServerSpec](k8s-api-apm-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-apm-v1beta1-apmserverspec)
* [ElasticsearchSpec](k8s-api-elasticsearch-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1beta1-elasticsearchspec)
* [KibanaSpec](k8s-api-kibana-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-kibana-v1beta1-kibanaspec)

::::


| Field | Description |
| --- | --- |
| **`secretName`** *string*<br> | SecretName is the name of the secret.<br> |
| **`entries`** *[KeyToPath](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-keytopath) array*<br> | Entries define how to project each key-value pair in the secret to filesystem paths. If not defined, all keys will be projected to similarly named paths in the filesystem. If defined, only the specified keys will be projected to the corresponding paths.<br> |


## SelfSignedCertificate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-selfsignedcertificate]

SelfSignedCertificate holds configuration for the self-signed certificate generated by the operator.

::::{admonition} Appears In:
* [TLSOptions](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-tlsoptions)

::::


| Field | Description |
| --- | --- |
| **`subjectAltNames`** *[SubjectAlternativeName](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-subjectalternativename) array*<br> | SubjectAlternativeNames is a list of SANs to include in the generated HTTP TLS certificate.<br> |
| **`disabled`** *boolean*<br> | Disabled indicates that the provisioning of the self-signed certifcate should be disabled.<br> |


## ServiceTemplate [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-servicetemplate]

ServiceTemplate defines the template for a Kubernetes Service.

::::{admonition} Appears In:
* [HTTPConfig](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-httpconfig)

::::


| Field | Description |
| --- | --- |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ServiceSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#servicespec-v1-core)*<br> | Spec is the specification of the service.<br> |


## SubjectAlternativeName [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-subjectalternativename]

SubjectAlternativeName represents a SAN entry in a x509 certificate.

::::{admonition} Appears In:
* [SelfSignedCertificate](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-selfsignedcertificate)

::::


| Field | Description |
| --- | --- |
| **`dns`** *string*<br> | DNS is the DNS name of the subject.<br> |
| **`ip`** *string*<br> | IP is the IP address of the subject.<br> |


## TLSOptions [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-tlsoptions]

TLSOptions holds TLS configuration options.

::::{admonition} Appears In:
* [HTTPConfig](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-httpconfig)

::::


| Field | Description |
| --- | --- |
| **`selfSignedCertificate`** *[SelfSignedCertificate](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-selfsignedcertificate)*<br> | SelfSignedCertificate allows configuring the self-signed certificate generated by the operator.<br> |
| **`certificate`** *[SecretRef](k8s-api-common-k8s-elastic-co-v1beta1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1beta1-secretref)*<br> | Certificate is a reference to a Kubernetes secret that contains the certificate and private key for enabling TLS. The referenced secret should contain the following:<br><br>* `ca.crt`: The certificate authority (optional).<br>* `tls.crt`: The certificate (or a chain).<br>* `tls.key`: The private key to the first certificate in the certificate chain.<br> |


