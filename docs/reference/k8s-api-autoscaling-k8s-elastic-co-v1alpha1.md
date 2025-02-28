---
mapped_pages:
  - https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-api-autoscaling-k8s-elastic-co-v1alpha1.html
applies_to:
  deployment:
    eck: all
---

# autoscaling.k8s.elastic.co/v1alpha1 [k8s-api-autoscaling-k8s-elastic-co-v1alpha1]

Package v1alpha1 contains API schema definitions for managing ElasticsearchAutoscaler resources.

* [ElasticsearchAutoscaler](k8s-api-autoscaling-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler)

## ElasticsearchAutoscaler [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler]

ElasticsearchAutoscaler represents an ElasticsearchAutoscaler resource in a Kubernetes cluster.

| Field | Description |
| --- | --- |
| **`apiVersion`** *string*<br> | `autoscaling.k8s.elastic.co/v1alpha1`<br> |
| **`kind`** *string*<br> | `ElasticsearchAutoscaler`<br> |
| **`metadata`** *[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#objectmeta-v1-meta)*<br> | Refer to Kubernetes API documentation for fields of `metadata`.<br> |
| **`spec`** *[ElasticsearchAutoscalerSpec](k8s-api-autoscaling-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec)*<br> |  |


## ElasticsearchAutoscalerSpec [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec]

ElasticsearchAutoscalerSpec holds the specification of an Elasticsearch autoscaler resource.

::::{admonition} Appears In:
* [ElasticsearchAutoscaler](k8s-api-autoscaling-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscaler)

::::


| Field | Description |
| --- | --- |
| **`elasticsearchRef`** *[ElasticsearchRef](k8s-api-autoscaling-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchref)*<br> |  |
| **`pollingPeriod`** *[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.32/#duration-v1-meta)*<br> | PollingPeriod is the period at which to synchronize with the Elasticsearch autoscaling API.<br> |


## ElasticsearchRef [k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchref]

ElasticsearchRef is a reference to an Elasticsearch cluster that exists in the same namespace.

::::{admonition} Appears In:
* [ElasticsearchAutoscalerSpec](k8s-api-autoscaling-k8s-elastic-co-v1alpha1.md#k8s-api-github-com-elastic-cloud-on-k8s-v2-pkg-apis-autoscaling-v1alpha1-elasticsearchautoscalerspec)

::::


| Field | Description |
| --- | --- |
| **`name`** *string*<br> | Name is the name of the Elasticsearch resource to scale automatically.<br> |


