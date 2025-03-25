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

## 3.0.0 [elastic-cloud-kubernetes-300-breaking-changes]

::::{dropdown} Enterprise search no longer available with 9.0.0
The standlaone Enterprise search, App Search and Workplace Search products remain available in maintenance mode and are no longer recommended for new search experiences. We recommend transitioning to our actively developed Elastic Stack tools to build new semantic and AI powered search experiences.There will be no stand alone Enterprise search 9.x image to update to.
For more information, check [Migrating to 9.x from Enterprise Search 8.x versions](https://www.elastic.co/guide/en/enterprise-search/8.18/upgrading-to-9-x.html).
**Impact**<br> Upgrade to 9.0.0 will be blocked due to stand alone Enterprise search 
**Action**<br> check [Migrating to 9.x from Enterprise Search 8.x versions](https://www.elastic.co/guide/en/enterprise-search/8.18/upgrading-to-9-x.html).
::::
