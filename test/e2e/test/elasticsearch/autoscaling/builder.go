// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package autoscaling

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

var _ test.Builder = &AutoscalingBuilder{}

// AutoscalingBuilder helps to build and update autoscaling policies.
type AutoscalingBuilder struct {
	t        *testing.T
	policies map[string]v1alpha1.AutoscalingPolicySpec
	es       types.NamespacedName
}

func (ab *AutoscalingBuilder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "ElasticsearchAutoscaler CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &autoscalingv1alpha1.ElasticsearchAutoscalerList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
	}
}

func (ab *AutoscalingBuilder) CreationTestSteps(k *test.K8sClient) test.StepList {
	autoscaler := ab.Build()
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				Name: "Creating an ElasticsearchAutoscaler cluster should succeed",
				Test: test.Eventually(func() error {
					return k.CreateOrUpdate(autoscaler)
				}),
			},
		})
}

func (ab *AutoscalingBuilder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

func (ab *AutoscalingBuilder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				Name: "ElasticsearchAutoscaler should exist and have a status",
				Test: test.Eventually(func() error {
					var autoscaler autoscalingv1alpha1.ElasticsearchAutoscaler
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(ab.Build()), &autoscaler); err != nil {
						return err
					}
					if autoscaler.Status.ObservedGeneration == nil {
						return errors.New("ObservedGeneration is not set")
					}
					if *autoscaler.Status.ObservedGeneration != autoscaler.Generation {
						return fmt.Errorf("expected ObservedGeneration %d, got %d", autoscaler.Generation, *autoscaler.Status.ObservedGeneration)
					}
					return nil
				}),
			},
		})
}

func (ab *AutoscalingBuilder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	autoscaler := ab.Build()
	var previousGeneration int64
	return test.StepList{
		test.Step{
			Name: "Applying the ElasticsearchAutoscaler mutation should succeed",
			Test: test.Eventually(func() error {
				var curAutoscaler autoscalingv1alpha1.ElasticsearchAutoscaler
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(autoscaler), &curAutoscaler); err != nil {
					return err
				}
				// Save the current generation
				previousGeneration = curAutoscaler.Generation
				// merge annotations
				if curAutoscaler.Annotations == nil {
					curAutoscaler.Annotations = make(map[string]string)
				}
				for k, v := range autoscaler.Annotations {
					curAutoscaler.Annotations[k] = v
				}
				// defensive copy as the spec struct contains nested objects like ucfg.Config that don't marshal/unmarshal
				// without losing type information making later comparisons with deepEqual fail.
				curAutoscaler.Spec = *autoscaler.Spec.DeepCopy()
				// may error-out with a conflict if the resource is updated concurrently
				// hence the usage of `test.Eventually`
				return k.Client.Update(context.Background(), &curAutoscaler)
			}),
		},
		test.Step{
			Name: "ElasticsearchAutoscaler status should be updated",
			Test: test.Eventually(func() error {
				var autoscaler autoscalingv1alpha1.ElasticsearchAutoscaler
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(ab.Build()), &autoscaler); err != nil {
					return err
				}
				if !(autoscaler.Generation > previousGeneration) {
					return errors.New("Generation has not been increased")
				}
				if autoscaler.Status.ObservedGeneration == nil {
					return errors.New("ObservedGeneration is not set")
				}
				if *autoscaler.Status.ObservedGeneration != autoscaler.Generation {
					return fmt.Errorf("expected ObservedGeneration %d, got %d", autoscaler.Generation, *autoscaler.Status.ObservedGeneration)
				}
				return nil
			}),
		},
	}
}

func (ab *AutoscalingBuilder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	autoscaler := ab.Build()
	return test.StepList{
		{
			Name: "Deleting ElasticsearchAutoscaler should return no error",
			Test: test.Eventually(func() error {
				err := k.Client.Delete(context.Background(), autoscaler)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		},
		{
			Name: "ElasticsearchAutoscaler should not be there anymore",
			Test: test.Eventually(func() error {
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(autoscaler), autoscaler)
				if err != nil {
					if apierrors.IsNotFound(err) {
						return nil
					}
					return errors.Wrap(err, "expected 404 not found API error here")
				}
				return nil
			}),
		},
	}
}

func (ab *AutoscalingBuilder) MutationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

func (ab *AutoscalingBuilder) SkipTest() bool {
	return false
}

func NewAutoscalingBuilder(t *testing.T, es types.NamespacedName) *AutoscalingBuilder {
	return &AutoscalingBuilder{
		t:        t,
		es:       es,
		policies: make(map[string]v1alpha1.AutoscalingPolicySpec),
	}
}

func (ab *AutoscalingBuilder) DeepCopy() *AutoscalingBuilder {
	if ab == nil {
		return nil
	}
	copy := &AutoscalingBuilder{
		t:  ab.t,
		es: ab.es,
	}
	copy.policies = make(map[string]v1alpha1.AutoscalingPolicySpec, len(ab.policies))
	for k, v := range ab.policies {
		copy.policies[k] = *v.DeepCopy()
	}
	return copy
}

// WithPolicy adds or replaces an autoscaling policy.
func (ab *AutoscalingBuilder) WithPolicy(policy string, roles []string, resources v1alpha1.AutoscalingResources) *AutoscalingBuilder {
	ab.policies[policy] = v1alpha1.AutoscalingPolicySpec{
		NamedAutoscalingPolicy: v1alpha1.NamedAutoscalingPolicy{
			Name: policy,
			AutoscalingPolicy: v1alpha1.AutoscalingPolicy{
				Roles:    roles,
				Deciders: make(map[string]v1alpha1.DeciderSettings),
			},
		},
		AutoscalingResources: resources,
	}
	if stringsutil.StringInSlice("ml", roles) {
		// Disable ML scale down delay
		ab.policies[policy].Deciders["ml"] = map[string]string{"down_scale_delay": "0"}
	}
	return ab
}

// WithFixedDecider sets a setting for the fixed decider on an already existing policy.
func (ab *AutoscalingBuilder) WithFixedDecider(policy string, fixedDeciderSettings map[string]string) *AutoscalingBuilder {
	policySpec, exists := ab.policies[policy]
	if !exists {
		ab.t.Fatalf("fixed decider must be set on an existing policy")
	}
	policySpec.Deciders["fixed"] = fixedDeciderSettings
	ab.policies[policy] = policySpec
	return ab
}

func (ab *AutoscalingBuilder) Build() *autoscalingv1alpha1.ElasticsearchAutoscaler {
	autoscaler := autoscalingv1alpha1.ElasticsearchAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "autoscaler-" + ab.es.Name,
			Namespace: ab.es.Namespace,
		},
	}
	for _, policySpec := range ab.policies {
		autoscaler.Spec.AutoscalingPolicySpecs = append(autoscaler.Spec.AutoscalingPolicySpecs, policySpec)
	}
	autoscaler.Spec.ElasticsearchRef.Name = ab.es.Name
	return &autoscaler
}
