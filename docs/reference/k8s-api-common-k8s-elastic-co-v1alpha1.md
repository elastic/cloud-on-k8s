---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-common-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# common.k8s.elastic.co/v1alpha1 [k8s-api-common-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for common types used by all resources.

## Condition [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-condition]

Condition represents Elasticsearch resource’s condition. **This API is in technical preview and may be changed or removed in a future release.**

::::{admonition} Appears In:
* [Conditions](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditions)

::::


| Field | Description |
| --- | --- |
| **`type`** *[ConditionType](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditiontype)*<br> |  |
| **`status`** *[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#conditionstatus-v1-core)*<br> |  |
| **`lastTransitionTime`** *[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)*<br> |  |
| **`message`** *string*<br> |  |


## ConditionType (string) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditiontype]

ConditionType defines the condition of an Elasticsearch resource.

::::{admonition} Appears In:
* [Condition](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-condition)

::::



## Conditions ([Condition](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-condition)) [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditions]

::::{admonition} Appears In:
* [ElasticsearchStatus](k8s-api-elasticsearch-k8s-elastic-co-v1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-elasticsearch-v1-elasticsearchstatus)

::::


| Field | Description |
| --- | --- |
| **`type`** *[ConditionType](k8s-api-common-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-common-v1alpha1-conditiontype)*<br> |  |
| **`status`** *[ConditionStatus](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#conditionstatus-v1-core)*<br> |  |
| **`lastTransitionTime`** *[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#time-v1-meta)*<br> |  |
| **`message`** *string*<br> |  |
