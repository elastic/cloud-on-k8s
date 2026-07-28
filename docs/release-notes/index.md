---
navigation_title: "Elastic Cloud on Kubernetes"
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/release-highlights.html
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/eck-release-notes.html
---

# Elastic Cloud on Kubernetes release notes [elastic-cloud-kubernetes-release-notes]

Review the changes, fixes, and more in each release of Elastic Cloud on Kubernetes.

## 3.5.0 [elastic-cloud-kubernetes-350-release-notes]

### Release Highlights

#### Dynamic namespaces

ECK now supports label-selector-based namespace scoping as an alternative to the static list of managed namespaces. When `namespaceSelector` is configured, the operator evaluates a [Kubernetes label selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors) against namespace labels at runtime: when a namespace gains matching labels it is on-boarded immediately and the operator begins managing its Elastic resources; when a namespace's labels no longer match, it is off-boarded and the operator stops reconciling its resources. Both transitions happen live — no operator restart is required. This makes it straightforward to grow or shrink the set of managed namespaces by relabeling them, without any operator configuration changes. Dynamic namespace handling is an Enterprise feature. For more details, refer to the [dynamic namespace handling documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/dynamic-namespace-handling.md).

#### Pause orchestration annotation for maintenance windows

ECK now supports a `eck.k8s.elastic.co/pause-orchestration` annotation that temporarily suspends spec-driven orchestration on any ECK-managed resource. This is useful during maintenance windows — such as draining Kubernetes nodes or applying infrastructure changes — where you want to prevent ECK from applying spec changes while keeping essential housekeeping running. Unlike the existing `eck.k8s.elastic.co/managed: "false"` annotation (which stops all reconciliation entirely and is now deprecated), pausing orchestration keeps certificate rotation, service reconciliation, user and secret management, and health monitoring active, avoiding cluster degradation during extended pauses. The annotation is supported on all ECK-managed resource types, and its value is validated by the webhook. When orchestration is paused, ECK sets an `OrchestrationPaused` condition on the resource status; on resume, any pending spec changes are applied immediately. For more details, refer to the [pause orchestration documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/k8s-pause-orchestration.md).

#### Mutual TLS expansion across all Stack components

ECK 3.4 introduced {{es}} client certificate authentication with support for {{product.kibana}} only, and promised that support for the remaining components would follow. ECK 3.5 delivers on that: all remaining Stack components that connect to {{es}} — APM Server, Beats, Enterprise Search, Elastic Maps Server, Logstash, Elastic Agent (standalone), and Fleet Server — now automatically receive ECK-managed client certificates and present them when connecting to {{es}}. For fleet-managed agents, Fleet Server propagates the client certificate information to all connected agents automatically, with no additional configuration required.

ECK 3.5 also introduces a second, independent mTLS capability: Fleet Server can now be configured to require client certificates from connecting Elastic Agents, enforcing mutual TLS on the Fleet Server to Elastic Agent connection. This is an Enterprise feature and requires Fleet Server version 8.19.19+, 9.3.8+, 9.4.4+, or 9.5.0+.

For more details, refer to the [{{es}} client certificate authentication documentation](docs-content://deploy-manage/security/k8s-es-client-certificate-auth.md) and the [Fleet Server client certificate authentication documentation](docs-content://deploy-manage/security/k8s-fleet-server-client-certificate-auth.md).

#### AutoOps agent collector configuration

The `AutoOpsAgentPolicy` resource now exposes `spec.config` and `spec.configRef` fields for tuning the AutoOps agent's collector configuration directly from the CRD, without manually editing configuration files. This gives you control over which metricsets are collected and at what interval. For more details, refer to the [AutoOps data collection documentation](docs-content://deploy-manage/monitor/autoops/autoops-disable-metrics-collection.md).

#### Hot-reload of secure settings without pod restarts

For {{es}} 9.5 and later, ECK now supports an opt-in file-based delivery mechanism for `spec.secureSettings` that eliminates the rolling restart previously required on every secret update. Enable it by adding the `eck.k8s.elastic.co/file-based-secure-settings: "true"` annotation to your {{es}} resource; ECK then writes secrets directly into the {{es}} file-based settings path and {{es}} reloads them in place. For more details, refer to the [secure settings documentation](docs-content://deploy-manage/security/k8s-secure-settings.md#k8s-es-secure-settings-hot-reload).

#### StackConfigPolicy enhancements

ECK 3.5 adds two new capabilities to `StackConfigPolicy`. The new `securityRoles` field lets you define custom {{es}} roles declaratively within a policy and apply them consistently across all targeted clusters; ECK merges the definitions into the `roles.yml` file mounted on each {{es}} pod and {{es}} hot-reloads them without a pod restart. The new `variablesFrom` field lets you load key-value pairs from ConfigMaps and Secrets as substitution variables, referenced as `${VAR}` or `${VAR:-default}` expressions in the policy's `elasticsearch` and `kibana` fields, so a single policy definition can be reused across environments with different values. ECK watches all referenced sources and reconciles automatically when they change. For more details, refer to the [StackConfigPolicy documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/elastic-stack-configuration-policies.md).

#### Reduced operator memory footprint

ECK 3.5 ships two complementary improvements to reduce the operator's memory usage in large clusters. The controller-runtime cache is now automatically scoped to only watch core workload resources (Pods, StatefulSets, Deployments, DaemonSets, PodDisruptionBudgets) that carry the ECK type label, avoiding the cost of caching unrelated workloads running in the same cluster. An additional opt-in flag, `--label-based-discovery`, further narrows the cache for Secrets, Services, and ConfigMaps to those explicitly labelled with `eck.k8s.elastic.co/watched=true`, which significantly reduces memory and API server load in clusters with large numbers of user-managed resources of those types.

### Features and enhancements [elastic-cloud-kubernetes-350-features-and-enhancements]

- Implement Logstash support for presenting client certificates to {{es}} [#9308](https://github.com/elastic/cloud-on-k8s/pull/9308)
- Implement Beats support for presenting client certificates to {{es}} [#9306](https://github.com/elastic/cloud-on-k8s/pull/9306)
- Implement Enterprise Search support for presenting client certificates to {{es}} [#9332](https://github.com/elastic/cloud-on-k8s/pull/9332)
- Implement APM Server support for presenting client certificates to {{es}} [#9307](https://github.com/elastic/cloud-on-k8s/pull/9307)
- Implement Elastic Maps Server support for presenting client certificates to {{es}} [#9331](https://github.com/elastic/cloud-on-k8s/pull/9331)
- Implement ECK monitoring support for presenting client certificates to {{es}} [#9334](https://github.com/elastic/cloud-on-k8s/pull/9334)
- Implement Fleet Server and Elastic Agent support for presenting client certificates to {{es}} [#9234](https://github.com/elastic/cloud-on-k8s/pull/9234)
- Implement AutoOps agent support for presenting client certificates to {{es}} [#9333](https://github.com/elastic/cloud-on-k8s/pull/9333)
- Implement mTLS support for Fleet Server and Elastic Agent connections [#9399](https://github.com/elastic/cloud-on-k8s/pull/9399)
- Version-gate Fleet Server mTLS support [#9486](https://github.com/elastic/cloud-on-k8s/pull/9486)
- Add `pause-orchestration` annotation support for {{es}} [#9330](https://github.com/elastic/cloud-on-k8s/pull/9330)
- Update `annotator.sh` script for the `eck.k8s.elastic.co/pause-orchestration` annotation [#9354](https://github.com/elastic/cloud-on-k8s/pull/9354)
- Add `pause-orchestration` annotation support for stateless resources, Elastic Agent, and Beats [#9417](https://github.com/elastic/cloud-on-k8s/pull/9417)
- Add webhook validation for the `pause-orchestration` annotation [#9474](https://github.com/elastic/cloud-on-k8s/pull/9474)
- Add `pause-orchestration` annotation support for Logstash [#9484](https://github.com/elastic/cloud-on-k8s/pull/9484)
- Add `pause-orchestration` annotation support for AutoOps [#9477](https://github.com/elastic/cloud-on-k8s/pull/9477)
- Reduce operator memory footprint by configuring cache to only watch ECK-labelled resources [#9339](https://github.com/elastic/cloud-on-k8s/pull/9339)
- Introduce `--label-based-discovery` flag to narrow cache for Secrets, Services, and ConfigMaps [#9359](https://github.com/elastic/cloud-on-k8s/pull/9359)
- Simplified container resources spec for all ECK CRDs [#9346](https://github.com/elastic/cloud-on-k8s/pull/9346)
- [AutoOps] Add `sending_queue` configuration to configmap [#9391](https://github.com/elastic/cloud-on-k8s/pull/9391)
- Allow overriding AutoOps agent collector configuration via `spec.config`/`spec.configRef` [#9507](https://github.com/elastic/cloud-on-k8s/pull/9507)
- Add support for {{product.kibana}} Spaces in Fleet integration policies [#9410](https://github.com/elastic/cloud-on-k8s/pull/9410)
- Publish `olm.skipRange` to support sparse and air-gapped OLM catalogs [#9451](https://github.com/elastic/cloud-on-k8s/pull/9451)
- Support {{es}} role definitions in StackConfigPolicy [#9442](https://github.com/elastic/cloud-on-k8s/pull/9442)
- Introduce dynamic substitution variables for StackConfigPolicy [#9541](https://github.com/elastic/cloud-on-k8s/pull/9541)
- Validate secure settings sources against active StackConfigPolicies [#9593](https://github.com/elastic/cloud-on-k8s/pull/9593)
- {{product.kibana}} readiness probes use status API [#9468](https://github.com/elastic/cloud-on-k8s/pull/9468)
- Add map support for `extraObjects` in Helm charts [#9478](https://github.com/elastic/cloud-on-k8s/pull/9478)
- Opt-in support for file-based cluster settings enabling hot-reload of secure settings without pod restarts [#9458](https://github.com/elastic/cloud-on-k8s/pull/9458)
- Switch to Go native FIPS with a static binary [#9538](https://github.com/elastic/cloud-on-k8s/pull/9538)
- Dynamic namespaces: label-selector-based namespace scoping (Enterprise feature) [#9569](https://github.com/elastic/cloud-on-k8s/pull/9569)
- Relax custom CA secret parsing to support cert-manager secrets [#9574](https://github.com/elastic/cloud-on-k8s/pull/9574)
- Batch {{es}} keystore add-file invocations [#9440](https://github.com/elastic/cloud-on-k8s/pull/9440)
- Move Condition types from `common/v1alpha1` to `common/v1` [#9408](https://github.com/elastic/cloud-on-k8s/pull/9408)

### Fixes [elastic-cloud-kubernetes-350-fixes]

- Fix dynamic watch leak on AutoOps resource selector change [#9434](https://github.com/elastic/cloud-on-k8s/pull/9434)
- Fix unexpected pod restarts by scoping template hash computation to Spec [#9437](https://github.com/elastic/cloud-on-k8s/pull/9437)
- AutoOps: prefer `ca.crt` over `tls.crt` for {{es}} TLS verification [#9463](https://github.com/elastic/cloud-on-k8s/pull/9463)
- Fix Logstash ignoring `set-default-security-context` operator flag [#9551](https://github.com/elastic/cloud-on-k8s/pull/9551)
- Clean up service-account-token secrets on association Unbind [#9562](https://github.com/elastic/cloud-on-k8s/pull/9562)
- Verify owner references when building client cert trust bundle [#9561](https://github.com/elastic/cloud-on-k8s/pull/9561)
- Add missing RBAC for Kubernetes metricsets in agent ClusterRoles [#9612](https://github.com/elastic/cloud-on-k8s/pull/9612)
- Store `FLEET_SERVER_SERVICE_TOKEN` in Secret instead of plaintext pod env var [#9626](https://github.com/elastic/cloud-on-k8s/pull/9626)
- Gate Fleet Server minimum version to 8.13.0 for {{es}} mTLS support [#9598](https://github.com/elastic/cloud-on-k8s/pull/9598)

### Documentation improvements [elastic-cloud-kubernetes-350-documentation-improvements]

- Add auto section contents to reference sections, add redirect from old API page [#9426](https://github.com/elastic/cloud-on-k8s/pull/9426)
- Fix the documentation link on OperatorHub [#9420](https://github.com/elastic/cloud-on-k8s/pull/9420)

:::{dropdown} Updated dependencies

- Go 1.26.4 => 1.26.5
- cloud.google.com/go/auth v0.20.0 => v0.22.0
- cloud.google.com/go/storage v1.62.0 => v1.63.1
- github.com/Azure/azure-sdk-for-go/sdk/storage/azblob v1.6.4 => v1.8.0
- github.com/aws/aws-sdk-go-v2 v1.41.5 => v1.42.1
- github.com/aws/aws-sdk-go-v2/credentials v1.19.14 => v1.19.29
- github.com/aws/aws-sdk-go-v2/service/s3 v1.98.0 => v1.105.1
- github.com/gkampitakis/go-snaps v0.5.21 => v0.5.23
- github.com/google/go-containerregistry v0.21.4 => v0.21.7
- github.com/prometheus/common v0.67.5 => v0.70.0
- go.elastic.co/apm/module/apmelasticsearch/v2 v2.7.6 => v2.7.12
- go.elastic.co/apm/module/apmhttp/v2 v2.7.6 => v2.7.12
- go.elastic.co/apm/module/apmzap/v2 v2.7.6 => v2.7.12
- go.elastic.co/apm/v2 v2.7.6 => v2.7.12
- go.uber.org/zap v1.27.1 => v1.28.0
- golang.org/x/crypto v0.53.0 => v0.54.0
- golang.org/x/mod v0.36.0 => v0.37.0
- golang.org/x/sync v0.21.0 => v0.22.0
- golang.org/x/sys v0.46.0 => v0.47.0
- golang.org/x/term v0.44.0 => v0.45.0
- golang.org/x/text v0.38.0 => v0.40.0
- golang.org/x/tools v0.45.0 => v0.47.0
- google.golang.org/api v0.274.0 => v0.288.0
- google.golang.org/grpc v1.81.1 => v1.82.0
- k8s.io/api v0.35.3 => v0.36.2
- k8s.io/apimachinery v0.35.3 => v0.36.2
- k8s.io/client-go v0.35.3 => v0.36.2
- sigs.k8s.io/controller-runtime v0.23.3 => v0.24.1
- sigs.k8s.io/controller-tools v0.20.1 => v0.21.0

:::

## 3.4.1 [elastic-cloud-kubernetes-341-release-notes]

:::{dropdown} Updated dependencies

- Go 1.26.2 => 1.26.4
- github.com/cncf/xds/go v0.0.0-20251210132809-ee656c7534f5 => v0.0.0-20260202195803-dba9d589def2
- github.com/envoyproxy/go-control-plane/envoy v1.36.0 => v1.37.0
- github.com/envoyproxy/protoc-gen-validate v1.3.0 => v1.3.3
- github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.7 => v2.29.0
- github.com/moby/spdystream v0.5.0 => v0.5.1
- go.opentelemetry.io/contrib/detectors/gcp v1.39.0 => v1.42.0
- go.opentelemetry.io/otel v1.43.0 => v1.44.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.40.0 => v1.44.0
- go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.40.0 => v1.44.0
- go.opentelemetry.io/otel/metric v1.43.0 => v1.44.0
- go.opentelemetry.io/otel/sdk v1.43.0 => v1.44.0
- go.opentelemetry.io/otel/sdk/metric v1.43.0 => v1.44.0
- go.opentelemetry.io/otel/trace v1.43.0 => v1.44.0
- go.opentelemetry.io/proto/otlp v1.9.0 => v1.10.0
- golang.org/x/crypto v0.49.0 => v0.53.0
- golang.org/x/mod v0.34.0 => v0.36.0
- golang.org/x/net v0.52.0 => v0.56.0
- golang.org/x/sync v0.20.0 => v0.21.0
- golang.org/x/sys v0.42.0 => v0.46.0
- golang.org/x/term v0.41.0 => v0.44.0
- golang.org/x/text v0.35.0 => v0.38.0
- golang.org/x/tools v0.43.0 => v0.45.0
- google.golang.org/genproto/googleapis/api v0.0.0-20260401001100-f93e5f3e9f0f => v0.0.0-20260526163538-3dc84a4a5aaa
- google.golang.org/genproto/googleapis/rpc v0.0.0-20260401024825-9d38bb4040a9 => v0.0.0-20260526163538-3dc84a4a5aaa
- google.golang.org/grpc v1.80.0 => v1.81.1

:::

## 3.4.0 [elastic-cloud-kubernetes-340-release-notes]

### Release Highlights

#### {{es}} client certificate authentication support

ECK now supports configuring {{es}} to require client certificates for authentication. This allows you to enforce mutual TLS (mTLS) between clients and {{es}}, strengthening security by requiring both the client and server to present valid certificates. Currently, {{es}} and {{product.kibana}} support this feature - {{product.kibana}} can be configured to present client certificates when connecting to {{es}}. Support for the remaining components that connect to {{es}} (Beats, Elastic Agent, APM Server, Logstash, and so on) will follow in future releases. For more details, refer to the [client certificate authentication documentation](docs-content://deploy-manage/security/k8s-es-client-certificate-auth.md).

#### Rolling restarts of {{es}} clusters

ECK now supports triggering rolling restarts of {{es}} clusters through a new annotation-based mechanism. This enables operators to gracefully restart all nodes in a cluster without manual intervention, useful for troubleshooting. The [rolling restart documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/nodes-orchestration.md#cluster-rolling-restart) provides more details.

#### Simplified zone awareness configuration

ECK simplifies the configuration of zone awareness for {{es}} clusters, reducing the amount of boilerplate configuration needed to set up topology-aware allocation. For more details, refer to the [zone awareness documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/advanced-elasticsearch-node-scheduling.md#k8s-zone-awareness).

#### ECK container image signing

ECK container images are now signed using [Sigstore cosign](https://docs.sigstore.dev/cosign/). This allows users to verify the authenticity and integrity of ECK operator images before deployment, strengthening the supply chain security of their Kubernetes clusters.

#### Automatic password-protected keystore for {{es}} in FIPS mode

ECK now automatically manages a password-protected keystore for {{es}} when FIPS mode is enabled. When `xpack.security.fips_mode.enabled` is set to `true` in the {{es}} configuration, the operator generates, stores, and configures a password-protected keystore — eliminating the need for manual `podTemplate` overrides. This feature activates for {{es}} 9.4.0+ and respects any existing user-provided keystore password configuration. For more details, refer to the [{{es}} FIPS keystore password documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/deploy-fips-compatible-version-of-eck.md#k8s-fips-keystore-password).

### Features and enhancements [elastic-cloud-kubernetes-340-features-and-enhancements]

- Implement client certificate required support for {{es}} [#9229](https://github.com/elastic/cloud-on-k8s/pull/9229)
- Implement {{product.kibana}} support for presenting client certificates to {{es}} [#9230](https://github.com/elastic/cloud-on-k8s/pull/9230)
- Support rolling restarts of {{es}} clusters [#9172](https://github.com/elastic/cloud-on-k8s/pull/9172)
- Simplify zone awareness [#9148](https://github.com/elastic/cloud-on-k8s/pull/9148)
- Operator-managed FIPS keystore password support for {{es}} [#9287](https://github.com/elastic/cloud-on-k8s/pull/9287) (issue: [#9171](https://github.com/elastic/cloud-on-k8s/issues/9171))
- Surface webhook warnings; Refactor webhooks to use controller-runtime's Validator [#9235](https://github.com/elastic/cloud-on-k8s/pull/9235)
- Add `extraObjects` support to ECK Helm charts [#9069](https://github.com/elastic/cloud-on-k8s/pull/9069)
- Add `kubeAPIServerPort` configuration option to Helm chart [#8980](https://github.com/elastic/cloud-on-k8s/pull/8980)
- Set `seccompProfile` to `RuntimeDefault` [#9012](https://github.com/elastic/cloud-on-k8s/pull/9012)
- Validate user-supplied HTTP CA certificate [#8992](https://github.com/elastic/cloud-on-k8s/pull/8992)
- Sign ECK container images (v2) [#9078](https://github.com/elastic/cloud-on-k8s/pull/9078)
- Improve license signature verification error to diagnose wrong license type [#9262](https://github.com/elastic/cloud-on-k8s/pull/9262)
- Improve AutoOpsAgentPolicy status reporting [#9095](https://github.com/elastic/cloud-on-k8s/pull/9095)
- Support `runAsNonRoot` true for recent versions of EPR [#8974](https://github.com/elastic/cloud-on-k8s/pull/8974)
- Reduce operator memory footprint by stripping managed fields from informer caches [#9321](https://github.com/elastic/cloud-on-k8s/pull/9321)
- Add version-gated querylog fileset to Filebeat sidecar config [#9291](https://github.com/elastic/cloud-on-k8s/pull/9291)
- Bump default {{product.kibana}} memory limit from 1Gi to 2Gi [#9328](https://github.com/elastic/cloud-on-k8s/pull/9328)
- Add image digest support to eck-operator Helm chart [#9362](https://github.com/elastic/cloud-on-k8s/pull/9362)

### Fixes [elastic-cloud-kubernetes-340-fixes]

- Prevent StackConfigPolicy controller from performing unnecessary file-settings secret updates on every reconciliation [#9316](https://github.com/elastic/cloud-on-k8s/pull/9316)
- Correct NetworkPolicy namespace selector label for soft multi-tenancy [#9153](https://github.com/elastic/cloud-on-k8s/pull/9153)
- Prevent using a nodeSet name while the equivalent StatefulSet already exists [#9036](https://github.com/elastic/cloud-on-k8s/pull/9036)
- Skip default PVC if volume with same name exists [#9199](https://github.com/elastic/cloud-on-k8s/pull/9199) (issue: [#8744](https://github.com/elastic/cloud-on-k8s/issues/8744))
- Avoid empty reconcile requests in StackConfigPolicy secret watch [#9179](https://github.com/elastic/cloud-on-k8s/pull/9179)
- Make remote-ca secret generation failures non-blocking [#9271](https://github.com/elastic/cloud-on-k8s/pull/9271)
- Garbage collect Agent soft-owned secrets on deletion [#9090](https://github.com/elastic/cloud-on-k8s/pull/9090)
- Detect stale CA in certificate chain and trigger certificates reissuance [#9197](https://github.com/elastic/cloud-on-k8s/pull/9197)
- Skip per-shard replica checks for GREEN clusters in `require_started_replica` predicate [#9188](https://github.com/elastic/cloud-on-k8s/pull/9188)
- Handle server side default for `TrafficDistribution` [#8994](https://github.com/elastic/cloud-on-k8s/pull/8994)
- Set default security context to {{product.kibana}} init container [#9218](https://github.com/elastic/cloud-on-k8s/pull/9218)
- Validate user-supplied CA for the transport layer of {{es}} [#8953](https://github.com/elastic/cloud-on-k8s/pull/8953)
- Align DaemonSet `UpdateReconciled` with Deployment reconciler [#9256](https://github.com/elastic/cloud-on-k8s/pull/9256) (issue: [#9246](https://github.com/elastic/cloud-on-k8s/issues/9246))

### Documentation improvements [elastic-cloud-kubernetes-340-documentation-improvements]

- Add recipe for manual mTLS configuration [#9124](https://github.com/elastic/cloud-on-k8s/pull/9124)
- Mention `PodTopologyLabelsAdmission` in {{es}} sample [#9035](https://github.com/elastic/cloud-on-k8s/pull/9035)
- Logstash Chart improvements [#9087](https://github.com/elastic/cloud-on-k8s/pull/9087)

:::{dropdown} Updated dependencies

- Go 1.25.8 => 1.26.2
- github.com/elastic/go-ucfg v0.8.9-0.20251017163010-3520930bed4f => v0.9.1
- github.com/gkampitakis/go-snaps v0.5.19 => v0.5.21
- github.com/google/go-containerregistry v0.20.7 => v0.21.4
- github.com/hashicorp/vault/api v1.22.0 => v1.23.0
- go.elastic.co/apm/v2 v2.7.2 => v2.7.6
- golang.org/x/crypto v0.46.0 => v0.49.0
- k8s.io/api v0.35.0 => v0.35.3
- k8s.io/apimachinery v0.35.0 => v0.35.3
- k8s.io/client-go v0.35.0 => v0.35.3
- k8s.io/klog/v2 v2.130.1 => v2.140.0
- sigs.k8s.io/controller-runtime v0.22.4 => v0.23.3
- sigs.k8s.io/controller-tools v0.20.0 => v0.20.1
- New direct dependencies: cloud.google.com/go/auth, cloud.google.com/go/storage, github.com/Azure/azure-sdk-for-go/sdk/storage/azblob, github.com/aws/aws-sdk-go-v2, google.golang.org/api

:::

## 3.3.2 [elastic-cloud-kubernetes-332-release-notes]

### Release Highlights

#### Fix ECK FIPS build

ECK 3.3.2 fixes the FIPS build by correctly enabling the BoringCrypto experiment via `GOEXPERIMENT=boringcrypto`. This release also adds preliminary support for native Go FIPS 140-3 mode (introduced in Go 1.24), which will be enabled in a future release once the module is certified.

### Features and enhancements [elastic-cloud-kubernetes-332-features-and-enhancements]

- Fix FIPS build and add native Go FIPS 140-3 support [#9263](https://github.com/elastic/cloud-on-k8s/pull/9263)

:::{dropdown} Updated dependencies

- Go 1.25.7 => 1.25.8

:::

## 3.3.1 [elastic-cloud-kubernetes-331-release-notes]

### Release Highlights

#### Removing Enterprise requirement for Elastic AutoOps

ECK 3.3.1 has removed the enterprise requirement for AutoOpsAgentPolicy. AutoOps can now be used by on premises users without the need for an enterprise license.

### Features and enhancements [elastic-cloud-kubernetes-331-features-and-enhancements]

- Removing enterprise requirement for AutoOpsAgentPolicy [#9125](https://github.com/elastic/cloud-on-k8s/pull/9125)
- Add Namespace Selector to AutoOpsAgentPolicy [#8991](https://github.com/elastic/cloud-on-k8s/pull/8991)
- Update minimum AutoOps Agent to 9.2.4 when a Basic license is used [#9157](https://github.com/elastic/cloud-on-k8s/pull/9157)

:::{dropdown} Updated dependencies

- Go 1.25.6 => 1.25.7
- github.com/elastic/go-ucfg v0.8.9-0.20251017163010-3520930bed4f -> v0.8.9-0.20260108155023-368693374ae9
- go.elastic.co/apm/v2 v2.7.2 -> v2.7.3
- golang.org/x/crypto v0.46.0 -> v0.48.0
- k8s.io/api v0.35.0 -> v0.35.1
- k8s.io/apimachinery v0.35.0 -> v0.35.1
- k8s.io/client-go v0.35.0 -> v0.35.1

:::

## 3.3.0 [elastic-cloud-kubernetes-330-release-notes]

### Release Highlights

#### AutoOps Integration (Enterprise feature)

ECK now supports integration with Elastic AutoOps through a new `AutoOpsAgentPolicy` custom resource. This allows you to instrument multiple {{es}} clusters at once for automated health monitoring and performance recommendations. The [AutoOps documentation](https://www.elastic.co/docs/deploy-manage/monitor/autoops.md) provides more details.

#### Elastic Package Registry Integration

ECK now supports deploying and managing Elastic Package Registry (EPR) through a new `PackageRegistry` custom resource. This is particularly useful for air-gapped environments, enabling {{product.kibana}} to reference a self-hosted registry instead of the public one. The [package registry documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/package-registry.md) provides more details.

#### Multiple Stack Configuration Policies composition support (Enterprise feature)

ECK now includes support for multiple Stack Config Policies targeting the same {{es}} cluster or {{product.kibana}} instance, using a weight-based priority system for deterministic policy composition. The [Stack Config Policy documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/elastic-stack-configuration-policies.md) provides more details.

### Features and enhancements [elastic-cloud-kubernetes-330-features-and-enhancements]

- AutoOpsAgentPolicy support [#8941](https://github.com/elastic/cloud-on-k8s/pull/8941) (issue: [#8789](https://github.com/elastic/cloud-on-k8s/issues/8789))
- ElasticPackageRegistry support [#8800](https://github.com/elastic/cloud-on-k8s/pull/8800) (issue: [#8925](https://github.com/elastic/cloud-on-k8s/issues/8925))
- Stack Config Policies composition support [#8917](https://github.com/elastic/cloud-on-k8s/pull/8917)
- Use standard {{product.kibana}} labels and Helm labels on the ECK Operator pod [#8840](https://github.com/elastic/cloud-on-k8s/pull/8840) (issue: [#8584](https://github.com/elastic/cloud-on-k8s/issues/8584))
- Add service customization support for {{es}} remote cluster server [#8892](https://github.com/elastic/cloud-on-k8s/pull/8892)
- Removal of {{es}} 6.x support from codebase [#8979](https://github.com/elastic/cloud-on-k8s/pull/8979)

### Fixes [elastic-cloud-kubernetes-330-fixes]

- Upgrade master StatefulSets last when performing a version upgrade of {{es}} [#8871](https://github.com/elastic/cloud-on-k8s/pull/8871) (issue: [#8429](https://github.com/elastic/cloud-on-k8s/issues/8429))
- Fix race condition for pre-existing Stack Config Policy [#8928](https://github.com/elastic/cloud-on-k8s/pull/8928) (issue: [#8912](https://github.com/elastic/cloud-on-k8s/issues/8912))
- Do not set {{product.kibana}} server.name [#8930](https://github.com/elastic/cloud-on-k8s/pull/8930) (issue: [#8929](https://github.com/elastic/cloud-on-k8s/issues/8929))
- Do not write `elasticsearch.k8s.elastic.co/managed-remote-clusters` when not necessary [#8932](https://github.com/elastic/cloud-on-k8s/pull/8932) (issue: [#8781](https://github.com/elastic/cloud-on-k8s/issues/8781))
- Cleanup orphaned secret mounts when removed from StackConfigPolicy [#8937](https://github.com/elastic/cloud-on-k8s/pull/8937) (issue: [#8921](https://github.com/elastic/cloud-on-k8s/issues/8921))
- Avoid duplicate error logging for generate GET operations on a GVK [#8957](https://github.com/elastic/cloud-on-k8s/pull/8957)
- Remove single master at a time upscale restriction [#8940](https://github.com/elastic/cloud-on-k8s/pull/8940) (issue: [#8939](https://github.com/elastic/cloud-on-k8s/issues/8939))


### Documentation improvements [elastic-cloud-kubernetes-330-documentation-improvements]

- Update Google Cloud LoadBalancer recipe for new requirements [#8843](https://github.com/elastic/cloud-on-k8s/pull/8843)
- Fix minUnavailable typo in PDB documentation [#8898](https://github.com/elastic/cloud-on-k8s/pull/8898)
- Use GKE ComputeClass instead of DaemonSet for GKE AutoPilot [#8982](https://github.com/elastic/cloud-on-k8s/pull/8982)
- Adjust `vm.max_map_count` to 1048576 in GKE AutoPilot recipes [#8986](https://github.com/elastic/cloud-on-k8s/pull/8986)
- Remove support for Stack 7.17. [#9038](https://github.com/elastic/cloud-on-k8s/pull/9038)

:::{dropdown} Updated dependencies

- Go 1.25.2 => 1.25.6
- github.com/KimMachineGun/automemlimit v0.7.4 => v0.7.5
- github.com/elastic/go-ucfg v0.8.9-0.20250307075119-2a22403faaea => v0.8.9-0.20251017163010-3520930bed4f
- github.com/gkampitakis/go-snaps v0.5.15 => v0.5.19
- github.com/google/go-containerregistry v0.20.6 => v0.20.7
- github.com/googlecloudplatform/compute-class-api => v0.0.0-20251208134148-ae2e7936c1f8
- github.com/prometheus/common v0.67.1 => v0.67.5
- github.com/spf13/cobra v1.10.1 => v1.10.2
- go.elastic.co/apm/v2 v2.7.1 => v2.7.2
- go.uber.org/zap v1.27.0 => v1.27.1
- golang.org/x/crypto v0.40.0 => v0.46.0
- k8s.io/api v0.34.1 => v0.35.0
- k8s.io/apimachinery v0.34.1 => v0.35.0
- k8s.io/client-go v0.34.1 => v0.35.0
- k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 => v0.0.0-20251002143259-bc988d571ff4
- sigs.k8s.io/controller-runtime v0.22.2 => v0.22.4
- sigs.k8s.io/controller-tools v0.19.0 => v0.20.0

:::

## 3.2.0 [elastic-cloud-kubernetes-320-release-notes]

### Release Highlights

#### Advanced PodDisruptionBudget management (Enterprise feature)

ECK now offers better out-of-the-box PodDisruptionBudgets that automatically keep your cluster available as Pods move across nodes. The new policy calculates the number of Pods per tier that can sustain replacement, and automatically generates a PodDisruptionBudget for each tier. This enables the Elasticsearch cluster to vacate Kubernetes nodes more quickly, while considering cluster health, without interruption. The documentation about [PodDisruptionBudget](docs-content://deploy-manage/deploy/cloud-on-k8s/pod-disruption-budget.md) has more information and details.

#### User Password Generation (Enterprise feature)

ECK now supports configuring the length of the generated password for the administrative user of each Elasticsearch cluster. While the default length remains 24 characters, this can now be configured up to a maximum of 72 characters. The password incorporates alphabetic and numeric characters to ensure strong complexity. Refer to the [managed credentials](docs-content://deploy-manage/users-roles/cluster-or-deployment-auth/managed-credentials-eck.md) page for examples and more details.

### Features and enhancements  [elastic-cloud-kubernetes-320-features-and-enhancements]

- Enable certificate reloading for stack monitoring Beats [#8833](https://github.com/elastic/cloud-on-k8s/pull/8833) (issue: [#5448](https://github.com/elastic/cloud-on-k8s/issues/5448))
- Allow configuration of file-based password character set and length [#8817](https://github.com/elastic/cloud-on-k8s/pull/8817) (issues: [#2795](https://github.com/elastic/cloud-on-k8s/issues/2795), [#8693](https://github.com/elastic/cloud-on-k8s/issues/8693))
- Automatically set GOMEMLIMIT based on cgroups memory limits [#8814](https://github.com/elastic/cloud-on-k8s/pull/8814) (issue: [#8790](https://github.com/elastic/cloud-on-k8s/issues/8790))
- Introduce granular PodDisruptionBudgets based on node roles [#8780](https://github.com/elastic/cloud-on-k8s/pull/8780) (issue: [#2936](https://github.com/elastic/cloud-on-k8s/issues/2936))

### Fixes  [elastic-cloud-kubernetes-320-fixes]

- Gate advanced Fleet config logic to Agent v8.13 and later [#8869](https://github.com/elastic/cloud-on-k8s/pull/8869)
- Ensure Agent configuration and state persist across restarts in Fleet mode [#8856](https://github.com/elastic/cloud-on-k8s/pull/8856) (issue: [#8819](https://github.com/elastic/cloud-on-k8s/issues/8819))
- Do not set credentials label on Kibana config secret [#8852](https://github.com/elastic/cloud-on-k8s/pull/8852) (issue: [#8839](https://github.com/elastic/cloud-on-k8s/issues/8839))
- Allow elasticsearchRef.secretName in Kibana helm validation [#8822](https://github.com/elastic/cloud-on-k8s/pull/8822) (issue: [#8816](https://github.com/elastic/cloud-on-k8s/issues/8816))

### Documentation improvements  [elastic-cloud-kubernetes-320-documentation-improvements]

- Update Logstash recipes from to filestream input [#8801](https://github.com/elastic/cloud-on-k8s/pull/8801)
- Recipe for exposing Fleet server to outside of the Kubernetes cluster [#8788](https://github.com/elastic/cloud-on-k8s/pull/8788)
- Clarify secretName restrictions [#8782](https://github.com/elastic/cloud-on-k8s/pull/8782)
- Update ES_JAVA_OPTS comments and explain auto-heap behavior [#8753](https://github.com/elastic/cloud-on-k8s/pull/8753)

### Miscellaneous  [elastic-cloud-kubernetes-320-miscellaneous]

:::{dropdown} Updated dependencies

- Go 1.24.5 => 1.25.2
- github.com/gkampitakis/go-snaps v0.5.13 => v0.5.15
- github.com/hashicorp/vault/api v1.20.0 => v1.22.0
- github.com/KimMachineGun/automemlimit => v0.7.4
- github.com/prometheus/client_golang v1.22.0 => v1.23.2
- github.com/prometheus/common v0.65.0 => v0.67.1
- github.com/sethvargo/go-password v0.3.1 => REMOVED
- github.com/spf13/cobra v1.9.1 => v1.10.1
- github.com/spf13/pflag v1.0.6 => v1.0.10
- github.com/spf13/viper v1.20.1 => v1.21.0
- github.com/stretchr/testify v1.10.0 => v1.11.1
- golang.org/x/crypto v0.40.0 => v0.43.0
- k8s.io/api v0.33.2 => v0.34.1
- k8s.io/apimachinery v0.33.2 => v0.34.1
- k8s.io/client-go v0.33.2 => v0.34.1
- k8s.io/utils v0.0.0-20241104100929-3ea5e8cea738 => v0.0.0-20250604170112-4c0f3b243397
- sigs.k8s.io/controller-runtime v0.21.0 => v0.22.2
- sigs.k8s.io/controller-tools v0.18.0 => v0.19.0
:::

## 3.1.0 [elastic-cloud-kubernetes-310-release-notes]

### Release Highlights

#### Propagate metadata to child Kubernetes resources

It is now possible to propagate metadata from the parent custom resource to the child resources created by the operator. If you add labels or annotations on an Elasticsearch, Kibana, or Agent resource, for example, these can be automatically propagated to the Pods, Services, and other resources created by the operator. Refer to the [Propagate Labels and Annotations](docs-content://deploy-manage/deploy/cloud-on-k8s/propagate-labels-annotations.md) page for examples and more details.

#### New UBI base image

To reduce the attack surface and improve overall security UBI images are now based on the UBI micro base image.

### Features and enhancements [elastic-cloud-kubernetes-310-features-and-enhancements]

- UBI: Use micro image instead of minimal [#8704](https://github.com/elastic/cloud-on-k8s/pull/8704)
- Propagate metadata to children [#8673](https://github.com/elastic/cloud-on-k8s/pull/8673) (issue: [#2652](https://github.com/elastic/cloud-on-k8s/issues/2652))
- Allow advanced configuration for fleet-managed Elastic Agents [#8623](https://github.com/elastic/cloud-on-k8s/pull/8623) (issue: [#8619](https://github.com/elastic/cloud-on-k8s/issues/8619))

### Fixes [elastic-cloud-kubernetes-310-fixes]

- Set owner on service account Secret, update it when application is recreated [#8716](https://github.com/elastic/cloud-on-k8s/pull/8716)
- fix: Cannot disable TLS in Logstash [#8706](https://github.com/elastic/cloud-on-k8s/pull/8706) (issue: [#8600](https://github.com/elastic/cloud-on-k8s/issues/8600))
- Move from deprecated container input to filestream [#8679](https://github.com/elastic/cloud-on-k8s/pull/8679) (issue: [#8667](https://github.com/elastic/cloud-on-k8s/issues/8667))
- Add automated workaround for 9.0.0 maps issue [#8665](https://github.com/elastic/cloud-on-k8s/pull/8665) (issue: [#8655](https://github.com/elastic/cloud-on-k8s/issues/8655))
- Bump go.mod to v3 [#8609](https://github.com/elastic/cloud-on-k8s/pull/8609)
- Helm: Add support for missing `remoteClusterServer` value [#8612](https://github.com/elastic/cloud-on-k8s/pull/8612)
- Add logs volume for Filebeat and Metricbeat in stack monitoring [#8606](https://github.com/elastic/cloud-on-k8s/pull/8606) (issue: [#8605](https://github.com/elastic/cloud-on-k8s/issues/8605))

### Documentation improvements [elastic-cloud-kubernetes-310-documentation-improvements]

- [Helm] Fix examples/logstash/basic-eck.yaml [#8695](https://github.com/elastic/cloud-on-k8s/pull/8695)

### Miscellaneous [elastic-cloud-kubernetes-310-miscellaneous]

:::{dropdown} Updated dependencies

- Update Go version to 1.24.5 [#8745](https://github.com/elastic/cloud-on-k8s/pull/8745)
- chore(deps): update registry.access.redhat.com/ubi9/ubi-micro docker tag to v9.6-1750858477 [#8711](https://github.com/elastic/cloud-on-k8s/pull/8711)
- fix(deps): update k8s to v0.33.2 [#8699](https://github.com/elastic/cloud-on-k8s/pull/8699)
- fix(deps): update module cloud.google.com/go/storage to v1.52.0 [#8629](https://github.com/elastic/cloud-on-k8s/pull/8629)
- fix(deps): update module github.com/go-git/go-git/v5 to v5.16.0 [#8631](https://github.com/elastic/cloud-on-k8s/pull/8631)
- fix(deps): update module github.com/google/go-containerregistry to v0.20.6 [#8672](https://github.com/elastic/cloud-on-k8s/pull/8672)
- fix(deps): update module github.com/magiconair/properties to v1.8.10 [#8625](https://github.com/elastic/cloud-on-k8s/pull/8625)
- fix(deps): update module github.com/prometheus/common to v0.63.0 [#8569](https://github.com/elastic/cloud-on-k8s/pull/8569)
- fix(deps): update module github.com/spf13/viper to v1.20.1 [#8570](https://github.com/elastic/cloud-on-k8s/pull/8570)
- fix(deps): update module google.golang.org/api to v0.227.0 [#8529](https://github.com/elastic/cloud-on-k8s/pull/8529)
- fix(deps): update module helm.sh/helm/v3 to 3.17.3 [#8598](https://github.com/elastic/cloud-on-k8s/pull/8598)
:::

## 3.0.0 [elastic-cloud-kubernetes-300-release-notes]

### Release Highlights

- ECK 3.0.0 adds support for Elastic Stack version 9.0.0. Elastic Stack version 9.0.0 is not supported on ECK operators running versions earlier than 3.0.0.

### Features and enhancements [elastic-cloud-kubernetes-300-features-enhancements]

- Add support for defining `dnsPolicy` and `dnsConfig` options for the ECK operator StatefulSet [#7999](https://github.com/elastic/cloud-on-k8s/pull/7999)
- Config: Allow escaping dots in keys via `[unsplit.key]` syntax [#8512](https://github.com/elastic/cloud-on-k8s/pull/8512) (issue: [#8499](https://github.com/elastic/cloud-on-k8s/issues/8499))
- Enable copying of ECK images to Amazon ECR to make it easier for users to find our own ECK operator in the AWS marketplace [#8427](https://github.com/elastic/cloud-on-k8s/pull/8427)
- Support new agent image path as of 9.0 [#8518](https://github.com/elastic/cloud-on-k8s/pull/8518)
- Remove ubi suffix for 9.x images [#8509](https://github.com/elastic/cloud-on-k8s/pull/8509)
- Remove support for 6.x Stack version [#8507](https://github.com/elastic/cloud-on-k8s/pull/8507)
- Log resourceVersion on Create and Update [#8503](https://github.com/elastic/cloud-on-k8s/pull/8503)
- Remove policyID validation [#8449](https://github.com/elastic/cloud-on-k8s/pull/8449) (issue: [#8446](https://github.com/elastic/cloud-on-k8s/issues/8446))
- Refactor APM server for 9.0.0 [#8448](https://github.com/elastic/cloud-on-k8s/pull/8448) (issue: [#8447](https://github.com/elastic/cloud-on-k8s/issues/8447))
- Improve error messages and events during Fleet setup [#8350](https://github.com/elastic/cloud-on-k8s/pull/8350)
- Validate updates to 9.0 go through 8.18 [#8559](https://github.com/elastic/cloud-on-k8s/pull/8559) (issue: [#8557](https://github.com/elastic/cloud-on-k8s/issues/8557))

### Fixes [elastic-cloud-kubernetes-300-fixes]

- Correctly parse managed namespaces when specified as an environment variable [#8513](https://github.com/elastic/cloud-on-k8s/pull/8513) (issue: [#7542](https://github.com/elastic/cloud-on-k8s/issues/7542))

### Documentation improvements [elastic-cloud-kubernetes-300-documentation-improvements]

- [DOCS] Updates release notes title ([#8599](https://github.com/elastic/cloud-on-k8s/pull/8599))
- Updates for Istio 1.24 ([#8476](https://github.com/elastic/cloud-on-k8s/pull/8476))
- Fix unresolved attribute in ECK Quickstart ([#8432](https://github.com/elastic/cloud-on-k8s/pull/8432))
- [Docs] Add synthetic monitoring example ([#8385](https://github.com/elastic/cloud-on-k8s/pull/8385)) (issue: [#6294](https://github.com/elastic/cloud-on-k8s/issues/6294))
- [docs] Update heap dump command to use the most recent Java process ([#8294](https://github.com/elastic/cloud-on-k8s/pull/8294))
- [DOC] Document the need for an ingest node for Enterprise Search analytics ([#8271](https://github.com/elastic/cloud-on-k8s/pull/8271))

### Miscellaneous [elastic-cloud-kubernetes-300-miscellaneous]

:::{dropdown} Updated dependencies

- chore(deps): update dependency go to v1.24.1 ([#8454](https://github.com/elastic/cloud-on-k8s/pull/8454))
- chore(deps): update docker.elastic.co/wolfi/go docker tag to v1.24 ([#8453](https://github.com/elastic/cloud-on-k8s/pull/8453))
- chore(deps): update registry.access.redhat.com/ubi9/ubi-minimal docker tag to v9.5-1741850109 ([#8544](https://github.com/elastic/cloud-on-k8s/pull/8544))
- fix(deps): update k8s to v0.32.2 ([#8486](https://github.com/elastic/cloud-on-k8s/pull/8486))
- fix(deps): update module github.com/gkampitakis/go-snaps to v0.5.11 ([#8524](https://github.com/elastic/cloud-on-k8s/pull/8524))
- fix(deps): update module github.com/go-git/go-git/v5 to v5.14.0 ([#8487](https://github.com/elastic/cloud-on-k8s/pull/8487))
- fix(deps): update module github.com/go-jose/go-jose/v4 from 4.0.1 to 4.0.5 ([#8488](https://github.com/elastic/cloud-on-k8s/pull/8488))
- fix(deps): update module github.com/google/go-cmp to v0.7.0 ([#8516](https://github.com/elastic/cloud-on-k8s/pull/8516))
- fix(deps): update module github.com/hashicorp/vault/api to v1.16.0 ([#8517](https://github.com/elastic/cloud-on-k8s/pull/8517))
- fix(deps): update module github.com/jonboulle/clockwork to v0.5.0 ([#8519](https://github.com/elastic/cloud-on-k8s/pull/8519))
- fix(deps): update module github.com/magiconair/properties to v1.8.9 ([#8307](https://github.com/elastic/cloud-on-k8s/pull/8307))
- fix(deps): update module github.com/prometheus/client_golang to v1.21.1 ([#8520](https://github.com/elastic/cloud-on-k8s/pull/8520))
- fix(deps): update module github.com/prometheus/common to v0.61.0 ([#8333](https://github.com/elastic/cloud-on-k8s/pull/8333))
- fix(deps): update module github.com/spf13/cobra to v1.9.1 ([#8523](https://github.com/elastic/cloud-on-k8s/pull/8523))
- fix(deps): update module github.com/spf13/pflag to v1.0.6 ([#8468](https://github.com/elastic/cloud-on-k8s/pull/8468))
- fix(deps): update module github.com/stretchr/testify to v1.10.0 ([#8282](https://github.com/elastic/cloud-on-k8s/pull/8282))
- fix(deps): update module go.elastic.co/apm/v2 to v2.7.0 ([#8576](https://github.com/elastic/cloud-on-k8s/pull/8576))
- fix(deps): update module golang.org/x/crypto from 0.29.0 to 0.31.0 ([#8334](https://github.com/elastic/cloud-on-k8s/pull/8334))
- fix(deps): update module golang.org/x/net package to 0.38.0 ([#8591](https://github.com/elastic/cloud-on-k8s/pull/8591))
- fix(deps): update module golang.org/x/oauth2 to v0.28.0 ([#8528](https://github.com/elastic/cloud-on-k8s/pull/8528))
- fix(deps): update module helm.sh/helm/v3 to v3.17.1 ([#8505](https://github.com/elastic/cloud-on-k8s/pull/8505))
- Update module github.com/gkampitakis/go-snaps to v0.5.10 ([#8467](https://github.com/elastic/cloud-on-k8s/pull/8467))
:::

