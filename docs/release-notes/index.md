---
navigation_title: "Elastic Cloud on Kubernetes"
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/release-highlights.html
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/eck-release-notes.html
---

# Elastic Cloud on Kubernetes release notes [elastic-cloud-kubernetes-release-notes]

Review the changes, fixes, and more in each release of Elastic Cloud on Kubernetes

## 3.3.0 [elastic-cloud-kubernetes-330-release-notes]

### Release Highlights

#### AutoOps Integration (Enterprise feature)

ECK now offers integration with Elastic AutoOps allowing users to easily integrate one or more {{es}} clusters into the solution to gain insight into common issues. The documentation for [AutoOps](https://www.elastic.co/docs/deploy-manage/monitor/autoops) has more information and details.

#### Elastic Package Registry Integration

ECK now supports deploying and managing Elastic Package Registry (EPR) as a custom resource. This enables users to run their own package registry for {{product.fleet}}, allowing {{product.kibana}} to reference a self-hosted registry instead of the public registry. The [package registry documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/epr) provide more details.

#### Multiple Stack Configuration Policies composition support (Enterprise feature)

ECK now includes support for multiple StackConfigPolicies (SCPs) targeting the same {{es}} cluster or {{kib}} instance, using a weight-based priority system for deterministic policy composition. The [stack config policy documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/elastic-stack-configuration-policies) provides more details.

### Features and enhancements  [elastic-cloud-kubernetes-330-features-and-enhancements]

- AutoOpsAgentPolicy support [#8941](https://github.com/elastic/cloud-on-k8s/pull/8941) (issue: [#8789](https://github.com/elastic/cloud-on-k8s/issues/8789))
- ElasticPackageRegistry support [#8800](https://github.com/elastic/cloud-on-k8s/pull/8800) (issue: [#8925](https://github.com/elastic/cloud-on-k8s/issues/8925))
- Stack Config Policies composition support [#8917](https://github.com/elastic/cloud-on-k8s/pull/8917)
- Use standard {{k8s}} labels and Helm labels on the ECK Operator pod [#8840](https://github.com/elastic/cloud-on-k8s/pull/8840) (issue: [#8584](https://github.com/elastic/cloud-on-k8s/issues/8584))
- Add service customization support for {{es}} remote cluster server [#8892](https://github.com/elastic/cloud-on-k8s/pull/8892)

### Fixes  [elastic-cloud-kubernetes-330-fixes]

- Upgrade master StatefulSets last when performing a version upgrade of {{es}} [#8871](https://github.com/elastic/cloud-on-k8s/pull/8871) (issue: [8429](https://github.com/elastic/cloud-on-k8s/issues/8429))
- Fix race condition for pre-existing Stack Config Policy [#8928](https://github.com/elastic/cloud-on-k8s/pull/8928) (issue: [#8912](https://github.com/elastic/cloud-on-k8s/issues/8912))
- Do not set {{kib}} server.name [#8930](https://github.com/elastic/cloud-on-k8s/pull/8930) (issue: [#8929](https://github.com/elastic/cloud-on-k8s/issues/8929))
- Do not write `elasticsearch.k8s.elastic.co/managed-remote-clusters` when not necessary [#8932](https://github.com/elastic/cloud-on-k8s/pull/8932) (issue: [#8781](https://github.com/elastic/cloud-on-k8s/issues/8781))
- Cleanup orphaned secret mounts when removed from StackConfigPolicy [#8937](https://github.com/elastic/cloud-on-k8s/pull/8937) (issue: [#8921](https://github.com/elastic/cloud-on-k8s/issues/8921))

### Documentation improvements  [elastic-cloud-kubernetes-330-documentation-improvements]

- Update Google Cloud LoadBalancer recipe for new requirements [#8843](https://github.com/elastic/cloud-on-k8s/pull/8843)
- Fix minUnavailable typo in PDB documentation [#8898](https://github.com/elastic/cloud-on-k8s/pull/8898)
- Use GKE ComputeClass instead of DaemonSet for GKE AutoPilot [#8982](https://github.com/elastic/cloud-on-k8s/pull/8982)
- Adjust `vm.max_map_count` to 1048576 in GKE AutoPilot recipes [#8986](https://github.com/elastic/cloud-on-k8s/pull/8986)

### Miscellaneous  [elastic-cloud-kubernetes-330-miscellaneous]

- Stop logging an error when returning an error when performing a generic GET operation on a GVK [#8957](https://github.com/elastic/cloud-on-k8s/pull/8957)

:::{dropdown} Updated dependencies

- Go 1.25.2 => 1.25.5
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

ECK now offers better out-of-the-box PodDisruptionBudgets that automatically keep your cluster available as Pods move across nodes. The new policy calculates the number of Pods per tier that can sustain replacement, and automatically generates a PodDisruptionBudget for each tier. This enables the {{es}} cluster to vacate Kubernetes nodes more quickly, while considering cluster health, without interruption. The documentation about [PodDisruptionBudget](docs-content://deploy-manage/deploy/cloud-on-k8s/pod-disruption-budget.md) has more information and details.

#### User Password Generation (Enterprise feature)

ECK now supports configuring the length of the generated password for the administrative user of each {{es}} cluster. While the default length remains 24 characters, this can now be configured up to a maximum of 72 characters. The password incorporates alphabetic and numeric characters to ensure strong complexity. Refer to the [managed credentials](docs-content://deploy-manage/users-roles/cluster-or-deployment-auth/managed-credentials-eck.md) page for examples and more details.

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