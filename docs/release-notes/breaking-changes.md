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

## 3.4.0 [elastic-cloud-kubernetes-340-breaking-changes]

::::{dropdown} Rolling restart of managed pods due to seccompProfile security context change
ECK 3.4.0 sets `seccompProfile` to `RuntimeDefault` on managed pods. This causes a rolling restart of nearly all ECK-managed pods ({{es}}, {{product.kibana}}, APM Server, Enterprise Search, Logstash, Elastic Maps Server, and Elastic Package Registry) during the operator upgrade. Beats and Elastic Agent pods are not affected. This rolling restart only occurs when the operator flag `set-default-security-context` is set to `auto-detect` (the default) or `true`.
For more information, check [PR #9012](https://github.com/elastic/cloud-on-k8s/pull/9012).

**Impact**<br> Upgrading to ECK 3.4.0 will trigger a rolling restart of most managed workloads. Plan the upgrade during a maintenance window.

**Action**<br> Schedule the ECK operator upgrade during a maintenance window to account for the rolling restart.
::::

::::{dropdown} Rolling restart of {{product.kibana}} pods due to init container security context change
ECK 3.4.0 sets a default security context on the {{product.kibana}} init container, which will cause {{product.kibana}} pods to rolling restart during the operator upgrade.
For more information, check [PR #9218](https://github.com/elastic/cloud-on-k8s/pull/9218).

**Impact**<br> {{product.kibana}} pods will be restarted as part of the operator upgrade.

**Action**<br> No action required. Be aware that {{product.kibana}} pods will restart during the upgrade.
::::

::::{dropdown} Rolling restart of {{es}} pods due to client certificate authentication support
ECK 3.4.0 adds client certificate authentication support for {{es}}. This changes the pre-stop hook and readiness probe scripts embedded in the {{es}} pod spec to handle client certificates when available, which causes a rolling restart of all {{es}} pods during the operator upgrade.
For more information, check [PR #9229](https://github.com/elastic/cloud-on-k8s/pull/9229) and [PR #9375](https://github.com/elastic/cloud-on-k8s/pull/9375).

**Impact**<br> All {{es}} pods will be restarted as part of the operator upgrade.

**Action**<br> No action required. Be aware that {{es}} pods will restart during the upgrade. Plan the upgrade during a maintenance window.
::::

::::{dropdown} Rolling restart of {{product.kibana}} pods due to default memory limit increase and potential OOM risk for low memory limits
ECK 3.4.0 increases the default {{product.kibana}} memory limit from 1Gi to 2Gi. {{product.kibana}} 9.4.x increased its V8 heap limit from 60% to 75% of container memory, but with 1Gi containers the resulting ~750MB of heap is not enough headroom for plugin initialization, leading to OOM crashes. This change will cause {{product.kibana}} pods to rolling restart during the operator upgrade.
For more information, check [PR #9328](https://github.com/elastic/cloud-on-k8s/pull/9328).

**Impact**<br> {{product.kibana}} pods that do not have explicit memory limits set will be restarted as part of the operator upgrade, and each pod will consume up to 2Gi of memory instead of 1Gi.

**Action**<br> Ensure that cluster nodes have sufficient memory to accommodate the increased default. If you have explicitly set a memory limit in the {{product.kibana}} `podTemplate`, this change does not affect you. However, if you have set a memory limit lower than 2Gi, be aware that {{product.kibana}} 9.4.0+ may experience OOM crashes due to the increased V8 heap usage.
::::

::::{dropdown} Default PVC handling change for {{es}} volumes
ECK 3.4.0 unifies how the operator handles default volume claim templates. Previously, the operator only skipped adding a default PVC when a non-PVC volume (such as `emptyDir` or `hostPath`) with the same name existed. Now, it skips the default PVC whenever any volume with the same name exists, including user-provided PVCs.
For more information, check [PR #9199](https://github.com/elastic/cloud-on-k8s/pull/9199).

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
