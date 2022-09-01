// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package autoscaling

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"

	autoscalingv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

// NewAutoscalingCapacityTest creates a step whose purpose is to validate that the Elasticsearch autoscaling API response
// does contain a non empty current/observed storage capacity for a policy with a data role.
// The observed capacity is not only used to decide when to scale up, but also to alert the user if the volume capacity
// is greater than the claimed capacity.
// See https://github.com/elastic/cloud-on-k8s/issues/4469 and https://github.com/elastic/cloud-on-k8s/pull/4493#discussion_r635869407
func NewAutoscalingCapacityTest(es esv1.Elasticsearch, k8sClient *test.K8sClient) test.Step {
	return test.Step{
		Name: "Autoscaling API response must contain the observed capacity",
		Test: test.Eventually(func() error {
			esClient, err := elasticsearch.NewElasticsearchClient(es, k8sClient)
			if err != nil {
				return err
			}
			capacity, err := esClient.GetAutoscalingCapacity(context.Background())
			if err != nil {
				return err
			}
			dataIngestPolicy, hasPolicy := capacity.Policies["data-ingest"]
			if !hasPolicy {
				return errors.New("Autoscaling policy \"data-ingest\" is expected in the autoscaling API response")
			}
			if dataIngestPolicy.CurrentCapacity.Total.Storage.IsZero() {
				return errors.New("Current total capacity for policy \"data-ingest\" should not be nil or 0")
			}
			if dataIngestPolicy.CurrentCapacity.Node.Storage.IsZero() {
				return errors.New("Current node capacity for policy \"data-ingest\" should not be nil or 0")
			}
			return nil
		}),
	}
}

// -- AutoscalingStatus Test Helper

type AutoscalingStatusTestBuilder struct {
	k                  *test.K8sClient
	ab                 *AutoscalingBuilder
	expectedConditions map[v1alpha1.ConditionType]bool
}

func (ab *AutoscalingBuilder) NewAutoscalingStatusTestBuilder(k8sClient *test.K8sClient) *AutoscalingStatusTestBuilder {
	return &AutoscalingStatusTestBuilder{
		expectedConditions: make(map[v1alpha1.ConditionType]bool),
		k:                  k8sClient,
		ab:                 ab,
	}
}

func (astb *AutoscalingStatusTestBuilder) ShouldBeActive() *AutoscalingStatusTestBuilder {
	astb.expectedConditions[v1alpha1.ElasticsearchAutoscalerActive] = true
	return astb
}

func (astb *AutoscalingStatusTestBuilder) ShouldBeOnline() *AutoscalingStatusTestBuilder {
	astb.expectedConditions[v1alpha1.ElasticsearchAutoscalerOnline] = true
	return astb
}

func (astb *AutoscalingStatusTestBuilder) ShouldBeHealthy() *AutoscalingStatusTestBuilder {
	astb.expectedConditions[v1alpha1.ElasticsearchAutoscalerHealthy] = true
	return astb
}

func (astb *AutoscalingStatusTestBuilder) ShouldBeLimited() *AutoscalingStatusTestBuilder {
	astb.expectedConditions[v1alpha1.ElasticsearchAutoscalerLimited] = true
	return astb
}

func (astb *AutoscalingStatusTestBuilder) ToStep() test.Step {
	return test.Step{
		Name: "ElasticsearchAutoscaler status subresource should be the expected one",
		Test: test.Eventually(func() error {
			var autoscaler autoscalingv1alpha1.ElasticsearchAutoscaler
			if err := astb.k.Client.Get(context.Background(), k8s.ExtractNamespacedName(astb.ab.Build()), &autoscaler); err != nil {
				return err
			}
			for _, conditionType := range []v1alpha1.ConditionType{
				v1alpha1.ElasticsearchAutoscalerActive,
				v1alpha1.ElasticsearchAutoscalerOnline,
				v1alpha1.ElasticsearchAutoscalerHealthy,
				v1alpha1.ElasticsearchAutoscalerLimited,
			} {
				_, expectedState := astb.expectedConditions[conditionType]
				idx := autoscaler.Status.Conditions.Index(conditionType)
				if idx < 0 {
					return errors.Errorf("condition type \"%s\" not found in ElasticsearchAutoscaler.Status.Conditions", conditionType)
				}
				condition := autoscaler.Status.Conditions[idx]
				if (expectedState && condition.Status != v1.ConditionTrue) || (!expectedState && condition.Status != v1.ConditionFalse) {
					return errors.Errorf("for condition type \"%s\", expected Status is %t, but current Status is %s", conditionType, expectedState, condition.Status)
				}
			}
			return nil
		}),
	}
}
