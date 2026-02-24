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
