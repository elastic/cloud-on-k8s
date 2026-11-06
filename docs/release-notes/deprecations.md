---
navigation_title: "Deprecations"
---

# Elastic Cloud on Kubernetes deprecations [elastic-cloud-kubernetes-deprecations]

Over time, certain Elastic functionality becomes outdated and is replaced or removed. To help with the transition, Elastic deprecates functionality for a period before removal, giving you time to update your applications.

Review the deprecated functionality for Elastic Cloud on Kubernetes. While deprecations have no immediate impact, we strongly encourage you update your implementation after you upgrade. To learn how to upgrade, check out [upgrade docs](docs-content://deploy-manage/upgrade/orchestrator/upgrade-cloud-on-k8s.md).

% ## Next version

% ::::{dropdown} Deprecation title
% Description of the deprecation.
% For more information, check [PR #](PR link).
% **Impact**<br> Impact of deprecation.
% **Action**<br> Steps for mitigating deprecation impact.
% ::::

## 3.6.0 [elastic-cloud-kubernetes-360-deprecations]

::::{dropdown} The `legacy` value of `--enforce-rbac-on-refs` is deprecated, and the behavior of `true` will change
The `--enforce-rbac-on-refs` operator flag now accepts the values `false`, `true`, `legacy`, and `all`. The value `legacy` is an alias for `true` and is deprecated as of 3.6.0. It will be removed in a future release. The bare flag without a value still maps to `true`.

With `true`, the operator checks RBAC on all cross-namespace associations. It removes associations that fail the check for direct or transitive {{es}} references, and only emits a `Warning` event for the other reference types. In a future release, `true` will become an alias for `all`, and the operator will remove every association that fails the check.

**Impact**<br> The values `true` and `legacy` continue to work in 3.6.0. When `true` becomes an alias for `all`, cross-namespace associations of any type whose `ServiceAccount` cannot access the referenced resource will be removed.

**Action**<br> Replace `legacy` with `true` or `all`. Review the `Warning` events that the operator emits for cross-namespace associations and update the RBAC rules, so that your setup is ready when `true` becomes an alias for `all`. For more details, refer to the [cross-namespace associations documentation](docs-content://deploy-manage/deploy/cloud-on-k8s/restrict-cross-namespace-resource-associations.md#k8s-restrict-cross-namespace-enforcement-modes).

For more information, check [PR #9756](https://github.com/elastic/cloud-on-k8s/pull/9756).
::::

::::{dropdown} The `autoops-token` key of the AutoOps connection Secret is no longer used
The AutoOps agent now authenticates with the `cloud-connected-mode-api-key` key of the Secret referenced by `spec.secretName` on the `AutoOpsAgentPolicy` resource. The `autoops-token` key is no longer required as of 3.6.0, and ECK no longer injects it into the agent pods as the `AUTOOPS_TOKEN` environment variable.

**Impact**<br> Secrets that still contain the `autoops-token` key remain valid. ECK ignores the key.

**Action**<br> Remove the `autoops-token` key from the Secret. Remove any reference to `${env:AUTOOPS_TOKEN}` from configuration overrides in `spec.config` or `spec.configRef`, because the variable no longer resolves.

For more information, check [PR #9849](https://github.com/elastic/cloud-on-k8s/pull/9849).
::::

## 3.5.0 [elastic-cloud-kubernetes-350-deprecations]

::::{dropdown} The `eck.k8s.elastic.co/managed` annotation is deprecated in favor of `eck.k8s.elastic.co/pause-orchestration`
The `eck.k8s.elastic.co/managed` annotation is deprecated as of 3.5.0 and will be removed in a future release.
**Impact**<br> The annotation continues to work in 3.5.0, but support will be dropped in a future release.
**Action**<br> Replace any usage of `eck.k8s.elastic.co/managed: "false"` with `eck.k8s.elastic.co/pause-orchestration: "true"`. Unlike `managed: "false"`, which stops all reconciliation entirely, `pause-orchestration: "true"` is a safer replacement that keeps essential housekeeping running (certificate rotation, services, users, and health monitoring) while pausing spec-driven orchestration.
::::

## 3.4.1 [elastic-cloud-kubernetes-341-deprecations]

There are no deprecations for ECK 3.4.1

## 3.4.0 [elastic-cloud-kubernetes-340-deprecations]

There are no deprecations for ECK 3.4.0

## 3.3.2 [elastic-cloud-kubernetes-332-deprecations]

There are no deprecations for ECK 3.3.2

## 3.3.1 [elastic-cloud-kubernetes-331-deprecations]

There are no deprecations for ECK 3.3.1

## 3.3.0 [elastic-cloud-kubernetes-330-deprecations]

7.17.X Elastic stack versions are deprecated as of 15 January 2026 and will be removed in a future release. Please see the [EOL policy](https://www.elastic.co/support/eol) for more details.

## 3.2.0 [elastic-cloud-kubernetes-320-deprecations]

There are no deprecations for ECK 3.2

## 3.1.0 [elastic-cloud-kubernetes-310-deprecations]

There are no deprecations for ECK 3.1

## 3.0.0 [elastic-cloud-kubernetes-300-deprecations]

There are no deprecations for ECK 3.0
