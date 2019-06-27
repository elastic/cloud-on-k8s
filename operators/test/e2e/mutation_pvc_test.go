// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestResourcesRollingUpgrade creates a 3 nodes cluster,
// then mutates it to a 3 nodes cluster with different resources.
// PVCs should be reused during the mutation.
func TestResourcesRollingUpgrade(t *testing.T) {
	// create a 3 nodes cluster
	initStack := stack.NewStackBuilder("resources-upgrade-pvc-reuse").
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		}).
		WithUpdateStrategy(v1alpha1.UpdateStrategy{
			ChangeBudget: &v1alpha1.ChangeBudget{
				MaxUnavailable: 1, // authorize one pod to go down for PVC reuse
			},
		})

	// mutate to the same cluster, but different resources
	mutatedStack := initStack.
		WithNoESTopology().
		WithESMasterDataNodes(3, corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("3Gi"),
				corev1.ResourceCPU:    resource.MustParse("2"),
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack, stack.MutationTestsOptions{
		ExpectedNewPods:  3,
		ExpectedPVCReuse: 3,
	})
}

// TestESConfigRollingUpgrade creates a 3 nodes cluster,
// then mutates it to a 3 nodes cluster with different configurations.
// PVCs should be reused during the mutation.
func TestESConfigRollingUpgrade(t *testing.T) {
	// create a 3 nodes cluster
	initStack := stack.NewStackBuilder("config-upgrade-pvc-reuse").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithESConfig(map[string]interface{}{"node.attr.foo": "bar"}).
		WithUpdateStrategy(v1alpha1.UpdateStrategy{
			ChangeBudget: &v1alpha1.ChangeBudget{
				MaxUnavailable: 1, // authorize one pod to go down for PVC reuse
			},
		})

	// mutate to the same cluster, but a different node.attr in the config
	mutatedStack := stack.NewStackBuilder("config-upgrade-pvc-reuse").
		WithESMasterDataNodes(3, stack.DefaultResources).
		WithESConfig(map[string]interface{}{"node.attr.foo": "baz"}).
		WithUpdateStrategy(v1alpha1.UpdateStrategy{
			ChangeBudget: &v1alpha1.ChangeBudget{
				MaxUnavailable: 1, // authorize one pod to go down for PVC reuse
			},
		})

	stack.RunCreationMutationDeletionTests(t, initStack, mutatedStack, stack.MutationTestsOptions{
		ExpectedNewPods:  3,
		ExpectedPVCReuse: 3,
	})
}
