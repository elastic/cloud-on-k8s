---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-dependencies.html
navigation_title: ECK configuration flags
applies_to:
  deployment:
    eck: all
---

# {{eck}} configuration flags [k8s-operator-config-flags]

The following table lists and describes all the available configuration flags for the ECK operator. For details on how to include them in the provided ConfigMap, or use other configuration methods, refer to [ECK configuration](docs-content://deploy-manage/deploy/cloud-on-k8s/configure-eck.md).

| Flag | Default | Description |
| --- | --- | --- |
| `ca-cert-rotate-before` | `24h` | Duration representing how long before expiration CA certificates should be re-issued. |
| `ca-cert-validity` | `8760h` | Duration representing the validity period of a generated CA certificate. |
| `ca-dir` | `""` | Path to a directory containing a CA certificate (tls.crt) and its associated private key (tls.key) to be used for all managed resources. Effectively disables the CA rotation and validity options. |
| `cert-rotate-before` | `24h` | Duration representing how long before expiration TLS certificates should be re-issued. |
| `cert-validity` | `8760h` | Duration representing the validity period of a generated TLS certificate. |
| `config` | `""` | Path to a file containing the operator configuration. |
| `container-registry` | `docker.elastic.co` | Container registry to use for pulling Elastic Stack container images. |
| `container-repository` | `""` | Container repository to use for pulling Elastic Stack container images. |
| `container-suffix` | `""` | Suffix to be appended to container images by default. Cannot be combined with `--ubi-only` flag. |
| `disable-config-watch` | `false` | Watch the configuration file for changes and restart to apply them. Only effective when the `--config` flag is used to set the configuration file. |
| `disable-telemetry` | `false` | Disable periodically updating ECK telemetry data for Kibana to consume. |
| `elasticsearch-client-timeout` | `180s` | Default timeout for requests made by the Elasticsearch client. |
| `enable-leader-election` | `true` | Enable leader election. Must be set to true if using multiple replicas of the operator |
| `enable-tracing` | `false` | Enable APM tracing in the operator process. Use environment variables to configure APM server URL, credentials, and so on. Check [Apm Go Agent reference](apm-agent-go://reference/configuration.md) for details. |
| `enable-webhook` | `false` | Enables a validating webhook server in the operator process. |
| `enforce-rbac-on-refs` | `false` | Enables restrictions on cross-namespace resource association through RBAC. |
| `exposed-node-labels` | `""` | List of Kubernetes node labels which are allowed to be copied as annotations on the Elasticsearch Pods. Check [Topology spread constraints and availability zone awareness](docs-content://deploy-manage/deploy/cloud-on-k8s/advanced-elasticsearch-node-scheduling.md#k8s-availability-zone-awareness) for more details. |
| `ip-family` | `""` | Set the IP family to use. Possible values: IPv4, IPv6, "" (= auto-detect) |
| `kube-client-qps` | `0` | Set the maximum number of queries per second to the Kubernetes API. Default value is inherited from the [Go client](https://github.com/kubernetes/client-go/blob/e6538dd42b4fe55b6c754e41c66b43133ba41a59/rest/config.go#L44). |
| `kube-client-timeout` | `60s` | Set the request timeout for Kubernetes API calls made by the operator. |
| `log-verbosity` | `0` | Verbosity level of logs. `-2`=Error, `-1`=Warn, `0`=Info, `0` and above=Debug. |
| `manage-webhook-certs` | `true` | Enables automatic webhook certificate management. |
| `max-concurrent-reconciles` | `3` | Maximum number of concurrent reconciles per controller (Elasticsearch, Kibana, APM Server). Affects the ability of the operator to process changes concurrently. |
| `metrics-cert-dir` | `"{{TempDir}}/k8s-metrics-server/serving-certs"` | Location of TLS certs for the metrics server. Directory needs to contain tls.key and tls.crt. If empty self-signed certificates are used. Only effective when combined with metrics-port and metrics-secure. |
| `metrics-host` | `0.0.0.0` | The host to which the operator should bind to serve metrics in the Prometheus format. Will be combined with metrics-port. |
| `metrics-port` | `0` | Prometheus metrics port. Set to 0 to disable the metrics endpoint. |
| `metrics-secure` | `false` | Enables TLS for the metrics server. Only effective combined with metrics-port. |
| `namespaces` | `""` | Namespaces in which this operator should manage resources. Accepts multiple comma-separated values. Defaults to all namespaces if empty or unspecified. |
| `operator-namespace` | `""` | Namespace the operator runs in. Required. |
| `password-hash-cache-size` | `5 x max-concurrent-reconciles` | Sets the size of the password hash cache. Caching is disabled if explicitly set to 0 or any negative value. |
| `password-length` | `24` | Length of generated file-based passwords (enterprise-only feature) |
| `set-default-security-context` | `auto-detect` | Enables adding a default Pod Security Context to Elasticsearch Pods in Elasticsearch `8.0.0` and later. `fsGroup` is set to `1000` by default to match Elasticsearch container default UID. This behavior might not be appropriate for OpenShift and PSP-secured Kubernetes clusters, so it can be disabled. |
| `ubi-only` | `false` | Use only UBI container images to deploy Elastic Stack applications. UBI images are only available from 7.10.0 onward. Ignored from 9.x as default images are based on UBI. Cannot be combined with `--container-suffix` flag. |
| `validate-storage-class` | `true` | Specifies whether the operator should retrieve storage classes to verify volume expansion support. Can be disabled if cluster-wide storage class RBAC access is not available. |
| `webhook-cert-dir` | `"{{TempDir}}/k8s-webhook-server/serving-certs"` | Path to the directory that contains the webhook server key and certificate. |
| `webhook-name` | `"elastic-webhook.k8s.elastic.co"` | Name of the Kubernetes ValidatingWebhookConfiguration resource. Only used when `enable-webhook` is true. |
| `webhook-secret` | `""` | K8s secret mounted into the path designated by webhook-cert-dir to be used for webhook certificates. |
| `webhook-port` | `9443` | Port to listen for incoming validation requests. |

Duration values should be specified as numeric values suffixed by the time unit. For example, a duration of 10 hours should be specified as `10h`. Acceptable time unit suffixes are:

| Suffix | Unit |
| --- | --- |
| `ms` | Milliseconds |
| `s` | Seconds |
| `m` | Minutes |
| `h` | Hours |
