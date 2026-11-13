---
navigation_title: "Breaking changes"
---

# Elastic Cloud on Kubernetes breaking changes [elastic-cloud-kubernetes-breaking-changes]
Breaking changes can impact your Elastic applications, potentially disrupting normal operations. Before you upgrade, carefully review the Elastic Cloud on Kubernetes breaking changes and take the necessary steps to mitigate any issues. To learn how to upgrade, check out [upgrade docs](docs-content://deploy-manage/upgrade/orchestrator/upgrade-cloud-on-k8s.md).

% ## Next version [elastic-cloud-kubernetes-nextversion-breaking-changes]

% ::::{dropdown} Title of breaking change
% Description of the breaking change.
% For more information, check [PR #](PR link).
% **Impact**<br> Impact of the breaking change.
% **Action**<br> Steps for mitigating deprecation impact.
% ::::

## 3.6.0 [elastic-cloud-kubernetes-360-breaking-changes]

::::{dropdown} Rolling restart of {{product.kibana}} pods, and of {{es}} pods when upgrading from 3.5.0
ECK 3.6.0 adds the `kibana.k8s.elastic.co/role` label to the pod template of every {{product.kibana}} resource to support background task isolation. This changes the pod spec, so {{product.kibana}} pods restart during the operator upgrade.

ECK 3.6.0 also removes the `eck.k8s.elastic.co/watched` label that ECK 3.5.0 leaked into the {{es}} and {{product.kibana}} pod templates. If you upgrade from 3.5.0, this changes the {{es}} pod spec and {{es}} pods restart as well. If you upgrade from 3.4.x or earlier, {{es}} pods do not restart.

**Impact**<br> All {{product.kibana}} pods will be restarted as part of the operator upgrade. All {{es}} pods will be restarted if the previous operator version is 3.5.0.

**Action**<br> No action required. Be aware that these pods will restart during the upgrade.

For more information, check [PR #9745](https://github.com/elastic/cloud-on-k8s/pull/9745) and [PR #9787](https://github.com/elastic/cloud-on-k8s/pull/9787).
::::

::::{dropdown} Rolling restart of Fleet Server and fleet-managed Elastic Agent pods during operator upgrade
ECK 3.6.0 includes fixes that modify the pod spec of Elastic Agent resources that run in Fleet mode. Duplicate volume mounts for the {{es}} CA and client certificate are removed. Fleet-managed agents that reference a Fleet Server in another namespace mount their copy of the {{es}} CA that the Fleet Server uses from a Secret with a new name, and the copy now contains only the `ca.crt` key.

**Impact**<br> All Fleet Server pods and all fleet-managed Elastic Agent pods will be restarted as part of the operator upgrade. Standalone Elastic Agent pods are not affected.

**Action**<br> No action required. Be aware that these pods will restart during the upgrade.

For more information, check [PR #9685](https://github.com/elastic/cloud-on-k8s/pull/9685), [PR #9686](https://github.com/elastic/cloud-on-k8s/pull/9686), and [PR #9703](https://github.com/elastic/cloud-on-k8s/pull/9703).
::::

::::{dropdown} RBAC enforcement for fleet-managed Elastic Agents that reach {{es}} through a Fleet Server in another namespace
ECK 3.6.0 fixes the RBAC check for the transitive path from a fleet-managed Elastic Agent to the {{es}} cluster that its Fleet Server references. Earlier versions copied the {{es}} CA and client certificates into the namespace of the Elastic Agent without this check. When the `--enforce-rbac-on-refs` operator flag is enabled, ECK now checks that the `ServiceAccount` of the Elastic Agent (`spec.serviceAccountName`, or `default` if not set) can access the {{es}} resource before it copies any Secret.

**Impact**<br> This affects only installations that enable `--enforce-rbac-on-refs`. After the operator upgrade, ECK removes the association of a fleet-managed Elastic Agent with its Fleet Server if the {{es}} cluster is in a different namespace than the Elastic Agent and the `ServiceAccount` of the Elastic Agent does not have `get` permission for that {{es}} resource. ECK also deletes the Secrets that it copied for this association.

**Action**<br> Before you upgrade, create the RBAC rules that allow the `ServiceAccount` of each fleet-managed Elastic Agent to access the {{es}} resource that its Fleet Server references. For more details, refer to the [cross-namespace associations documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/restrict-cross-namespace-resource-associations.md).

For more information, check [PR #9725](https://github.com/elastic/cloud-on-k8s/pull/9725).
::::

::::{dropdown} Rolling restart of Logstash pods that use API basic authentication
When `api.auth.type: basic` is configured, ECK 3.6.0 replaces the HTTP readiness probe, which embedded the API credentials in the StatefulSet spec, with a TCP socket probe. This changes the pod spec.

**Impact**<br> Logstash pods with `api.auth.type: basic` will be restarted as part of the operator upgrade. Other Logstash pods are not affected.

**Action**<br> No action required. Be aware that these pods will restart during the upgrade.

For more information, check [PR #9664](https://github.com/elastic/cloud-on-k8s/pull/9664).
::::

::::{dropdown} Restart of AutoOps agent pods and change of the default AutoOps agent image
ECK 3.6.0 changes the AutoOps agent in two ways. The agent now authenticates with the Cloud Connected Mode API key, so ECK no longer injects the `AUTOOPS_TOKEN` environment variable and no longer requires the `autoops-token` key in the configuration Secret. The default image also changes from `elastic-agent/elastic-otel-collector-wolfi` to `elastic-agent/elastic-otel-collector`, so that the `--container-suffix=-wolfi` operator flag selects the Wolfi variant like for all other stack images. The `-wolfi` suffix is also no longer applied to the Elastic Package Registry and Enterprise Search images, which are Wolfi-based by default.

**Impact**<br> All AutoOps agent pods will be restarted as part of the operator upgrade and will run the non-Wolfi image unless the `--container-suffix=-wolfi` operator flag (`config.containerSuffix=-wolfi` in the `eck-operator` Helm chart) is set. Configuration overrides that reference `${env:AUTOOPS_TOKEN}` no longer resolve.

**Action**<br> To keep the Wolfi image for the AutoOps agent, start the operator with `--container-suffix=-wolfi` (`config.containerSuffix=-wolfi` in the `eck-operator` Helm chart) or set `spec.image` on the `AutoOpsAgentPolicy` resource. Remove any reference to `${env:AUTOOPS_TOKEN}` from configuration overrides. Secrets that still contain the `autoops-token` key remain valid and the key is ignored, so you can remove it from the Secret referenced by `spec.secretName`.

For more information, check [PR #9849](https://github.com/elastic/cloud-on-k8s/pull/9849) and [PR #9663](https://github.com/elastic/cloud-on-k8s/pull/9663).
::::

## 3.5.0 [elastic-cloud-kubernetes-350-breaking-changes]

::::{dropdown} Rolling restart of Fleet-server pods during operator upgrade
ECK 3.5.0 includes changes that modify the Fleet-server pod spec, triggering a restart of all Fleet-server pods during the operator upgrade.

**Impact**<br> All Fleet-server pods will be restarted as part of the operator upgrade.

**Action**<br> No action required. Be aware that Fleet-server pods will restart during the upgrade.
::::

::::{dropdown} Rolling restart of {{es}} and {{product.kibana}} pods during operator upgrade
ECK 3.5.0 adds the `eck.k8s.elastic.co/watched: "true"` label to the Secrets, Services, and ConfigMaps that the operator manages. This label supports the new opt-in `--restrict-watched-resources` operator flag. Due to a bug, the label also leaked into the {{es}} and {{product.kibana}} pod templates. Because this changes their pod specs, their pods restart during the operator upgrade.

**Impact**<br> All {{es}} and {{product.kibana}} pods will be restarted as part of the operator upgrade. The `--restrict-watched-resources` flag is disabled by default and does not need to be enabled for this restart to occur.

**Action**<br> No action required. Be aware that {{es}} and {{product.kibana}} pods will restart during the upgrade. A future ECK release that fixes this bug removes the label again, which causes another restart of these pods.

For more information, check [PR #9359](https://github.com/elastic/cloud-on-k8s/pull/9359) and [Issue #9786](https://github.com/elastic/cloud-on-k8s/issues/9786).
::::

## 3.4.1 [elastic-cloud-kubernetes-341-breaking-changes]

There are no breaking changes for ECK 3.4.1

## 3.4.0 [elastic-cloud-kubernetes-340-breaking-changes]

::::{dropdown} Rolling restart of {{es}} pods during operator upgrade
ECK 3.4.0 includes changes that modify the {{es}} pod spec, triggering a rolling restart of all {{es}} pods during the operator upgrade. These changes include setting `seccompProfile` to `RuntimeDefault` and updated pre-stop hook and readiness probe scripts for client certificate authentication support.

**Impact**<br> All {{es}} pods will be restarted as part of the operator upgrade.

**Action**<br> No action required. Be aware that {{es}} pods will restart during the upgrade.
::::

::::{dropdown} Rolling restart of {{product.kibana}} pods during operator upgrade and potential OOM risk for low memory limits
ECK 3.4.0 includes changes that modify the {{product.kibana}} pod spec, triggering a rolling restart of {{product.kibana}} pods during the operator upgrade. These changes include setting `seccompProfile` to `RuntimeDefault`, a new default security context on the init container, and an increase of the default memory limit from 1Gi to 2Gi. The memory limit increase addresses OOM crashes for {{product.kibana}} 9.4.0+ where the 1Gi limit does not provide enough headroom for plugin initialization.

**Impact**<br> {{product.kibana}} pods will be restarted as part of the operator upgrade. Pods without explicit memory limits will consume up to 2Gi of memory instead of 1Gi.

**Action**<br> Ensure that cluster nodes have sufficient memory to accommodate the increased default. If you have explicitly set a memory limit in the {{product.kibana}} `podTemplate`, the memory limit change does not affect you. However, if you have set a memory limit lower than 2Gi, be aware that {{product.kibana}} 9.4.0+ may experience OOM crashes due to the increased V8 heap usage.
::::

::::{dropdown} Rolling restart of APM Server, Logstash, Elastic Maps Server, and Package Registry pods during operator upgrade
ECK 3.4.0 sets `seccompProfile: RuntimeDefault` on APM Server, Logstash, Elastic Maps Server, and Package Registry pods, triggering a rolling restart during the operator upgrade. This applies when `--set-default-security-context` is `true`, or `auto-detect` (the default) on non-OpenShift clusters. On OpenShift, `auto-detect` suppresses the injection.

**Impact**<br> These pods restart during the operator upgrade wherever the security context is injected (flag `true`, or `auto-detect` on non-OpenShift). OpenShift is unaffected.

**Action**<br> No action required on standard Kubernetes. On OpenShift, see [known issues](./known-issues.md) for a Logstash-specific case where the flag is incorrectly ignored.
::::

::::{dropdown} Default PVC handling change for {{es}} volumes
ECK 3.4.0 unifies how the operator handles default volume claim templates. Previously, the operator only skipped adding a default PVC when a non-PVC volume (such as `emptyDir` or `hostPath`) with the same name existed. Now, it skips the default PVC whenever any volume with the same name exists, including user-provided PVCs.

**Impact**<br> If you defined custom PVC volumes in `podTemplate.spec.volumes` with the same name as a default volume (for example `elasticsearch-data`), those custom volumes were previously ignored and default volumes were provisioned instead. After upgrading, the operator will attempt to use your custom PVC volumes, which may cause a StatefulSet update rejection by Kubernetes.

**Action**<br> If you encounter a StatefulSet update error after upgrading, remove the custom PVC entries from `podTemplate.spec.volumes` that overlap with default volume names.
::::


## 3.3.2 [elastic-cloud-kubernetes-332-breaking-changes]

There are no breaking changes for ECK 3.3.2

## 3.3.1 [elastic-cloud-kubernetes-331-breaking-changes]

There are no breaking changes for ECK 3.3.1

## 3.3.0 [elastic-cloud-kubernetes-330-breaking-changes]

There are no breaking changes for ECK 3.3

## 3.2.0 [elastic-cloud-kubernetes-320-breaking-changes]

There are no breaking changes for ECK 3.2

## 3.1.0 [elastic-cloud-kubernetes-310-breaking-changes]

There are no breaking changes for ECK 3.1

## 3.0.0 [elastic-cloud-kubernetes-300-breaking-changes]

::::{dropdown} Enterprise Search no longer available since version 9.0.0
The standalone Enterprise Search, App Search and Workplace Search products remain available in maintenance mode and are no longer recommended for new search experiences. We recommend transitioning to our actively developed Elastic Stack tools to build new semantic and AI powered search experiences. There will be no standalone Enterprise Search 9.x image to update to.
For more information, check [Migrating to 9.x from Enterprise Search 8.x versions](https://www.elastic.co/guide/en/enterprise-search/8.18/upgrading-to-9-x.html).

**Impact**<br> The upgrade to version 9.0.0 is not possible for standalone Enterprise search resources.

**Action**<br> Migrate away from Enterprise Search following [this guide](https://www.elastic.co/guide/en/enterprise-search/8.18/upgrading-to-9-x.html). Only once the standalone Enterprise Search resources have been deleted attempt the upgrade of the Elastic Stack to version 9.0.0.
::::
