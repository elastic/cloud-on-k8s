// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

func TestPauseOrchestration(t *testing.T) {
	esName := "test-pause-orchestration"
	esNamespace := test.Ctx().ManagedNamespace(0)

	// Start with pause-orchestration disabled (default)
	initialBuilder := elasticsearch.NewBuilder(esName).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	// Phase 1: transition to pause-orchestration: true
	enabledBuilder := initialBuilder.DeepCopy()
	enabledBuilder.Elasticsearch.Annotations = map[string]string{common.PauseOrchestrationAnnotation: "true"}
	enabledBuilder.MutatedFrom = &initialBuilder

	// Phase 2: update Elasticsearch node spec
	upscaleBuilder := enabledBuilder.DeepCopy().
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithMutatedFrom(enabledBuilder)

	// Phase 3: transition back to disabled
	disabledBuilder := upscaleBuilder.DeepCopy()
	disabledBuilder.Elasticsearch.Annotations[common.PauseOrchestrationAnnotation] = "false"
	disabledBuilder.MutatedFrom = &upscaleBuilder

	// Phase 4: transition back to enabled again
	reenabledBuilder := disabledBuilder.DeepCopy()
	reenabledBuilder.Elasticsearch.Annotations[common.PauseOrchestrationAnnotation] = "true"
	reenabledBuilder.MutatedFrom = disabledBuilder

	// Phase 5: delete a pod (below)

	// Phase 6: re-disable the annotation
	redisableBuilder := reenabledBuilder.DeepCopy()
	redisableBuilder.Elasticsearch.Annotations[common.PauseOrchestrationAnnotation] = "false"
	redisableBuilder.MutatedFrom = reenabledBuilder

	k := test.NewK8sClientOrFatal()

	// Use the builder's actual ES name (includes random suffix from NewBuilder).
	actualESName := initialBuilder.Elasticsearch.Name

	test.StepList{}.
		// Create with pause orchestration disabled (default)
		WithSteps(initialBuilder.InitTestSteps(k)).
		WithSteps(initialBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(initialBuilder, k)).
		WithSteps(verifyPauseOrchestrationDisabled(k, esNamespace, actualESName, 1)).
		// Phase 1: enable pause-orchestration annotation
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(initialBuilder, k)).
		WithSteps(enabledBuilder.MutationTestSteps(k)).
		WithSteps(verifyPauseOrchestrationEnabled(k, esNamespace, actualESName, 1, false)).
		// Phase 2: upscale Elasticsearch
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(upscaleBuilder, k)).
		WithSteps(upscaleBuilder.UpgradeTestSteps(k)).
		WithSteps(verifyPauseOrchestrationEnabled(k, esNamespace, actualESName, 1, true)).
		// Phase 3: disable pause-orchestration
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(*disabledBuilder, k)).
		WithSteps(disabledBuilder.UpgradeTestSteps(k)).
		WithSteps(disabledBuilder.RollingRestartTestSteps(k)).
		WithSteps(test.CheckTestSteps(disabledBuilder, k)).
		WithSteps(verifyPauseOrchestrationDisabled(k, esNamespace, actualESName, 3)).
		// Phase 4: re-enable pause-orchestration
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(*reenabledBuilder, k)).
		WithSteps(reenabledBuilder.MutationTestSteps(k)).
		WithSteps(verifyPauseOrchestrationEnabled(k, esNamespace, actualESName, 3, false)).
		// Phase 5: delete pod
		WithSteps(deletePod(k, esNamespace, actualESName)).
		WithSteps(test.CheckTestSteps(reenabledBuilder, k)).
		WithSteps(verifyPauseOrchestrationEnabled(k, esNamespace, actualESName, 3, false)).
		// Phase 6: re-disable pause-orchestration
		WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(*redisableBuilder, k)).
		WithSteps(redisableBuilder.UpgradeTestSteps(k)).
		WithSteps(redisableBuilder.RollingRestartTestSteps(k)).
		WithSteps(test.CheckTestSteps(redisableBuilder, k)).
		WithSteps(verifyPauseOrchestrationDisabled(k, esNamespace, actualESName, 3)).
		WithSteps(initialBuilder.DeletionTestSteps(k)).
		RunSequential(t)
}

func verifyPauseOrchestrationEnabled(k *test.K8sClient, namespace, esName string, expectedNodeCount int, specChangesMade bool) test.StepList {
	return test.StepList{
		{
			Name: fmt.Sprintf("Verify pause-orchestration annotation is set to true when spec changes made [%t]", specChangesMade),
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      esName,
				}, &es); err != nil {
					return err
				}
				if !common.IsOrchestrationPaused(&es) {
					return fmt.Errorf("annotation %s should be set to true", common.PauseOrchestrationAnnotation)
				}

				orchestrationPausedIndex := es.Status.Conditions.Index(esv1.OrchestrationPaused)
				if orchestrationPausedIndex < 0 {
					return fmt.Errorf("%s condition does not exist on Elasticsearch resource", esv1.OrchestrationPaused)
				}

				orchestrationCondition := es.Status.Conditions[orchestrationPausedIndex]
				if orchestrationCondition.Status == corev1.ConditionFalse {
					return fmt.Errorf("condition %s should be true", esv1.OrchestrationPaused)
				}

				if specChangesMade && orchestrationCondition.Message == "Orchestration paused via annotation; no pending spec changes detected" {
					return fmt.Errorf("condition message '%s' is incorrect when spec has changed", orchestrationCondition.Message)
				}

				if !specChangesMade && orchestrationCondition.Message == "Orchestration paused via annotation; spec changes are pending and will be applied on resume" {
					return fmt.Errorf("condition message '%s' is incorrect when spec has not changed", orchestrationCondition.Message)
				}

				return nil
			}),
		},
		{
			Name: "Verify expected number of pods are running",
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(test.ESPodListOptions(namespace, esName)...)
				if err != nil {
					return err
				}
				if len(pods) != expectedNodeCount {
					return fmt.Errorf("expected %d pods, got %d", expectedNodeCount, len(pods))
				}
				return nil
			}),
		},
	}
}

func verifyPauseOrchestrationDisabled(k *test.K8sClient, namespace, esName string, expectedNodeCount int) test.StepList {
	return test.StepList{
		{
			Name: "Verify pause-orchestration annotation is set to false",
			Test: test.Eventually(func() error {
				var es esv1.Elasticsearch
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      esName,
				}, &es); err != nil {
					return err
				}

				if common.IsOrchestrationPaused(&es) {
					return fmt.Errorf("annotation %s should be set to false", common.PauseOrchestrationAnnotation)
				}

				for _, condition := range es.Status.Conditions {
					if condition.Type == esv1.OrchestrationPaused && condition.Status == corev1.ConditionTrue {
						return fmt.Errorf("condition %s should be false", esv1.OrchestrationPaused)
					}
				}
				return nil
			}),
		},
		{
			Name: "Verify all expected pods are running",
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(test.ESPodListOptions(namespace, esName)...)
				if err != nil {
					return err
				}
				if len(pods) != expectedNodeCount {
					return fmt.Errorf("expected %d pods, got %d", expectedNodeCount, len(pods))
				}
				return nil
			}),
		},
	}
}

func deletePod(k *test.K8sClient, namespace, esName string) test.StepList {
	return test.StepList{
		{
			Name: "A new pod becomes ready when a pod is deleted",
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(test.ESPodListOptions(namespace, esName)...)
				if err != nil {
					return err
				}
				if len(pods) == 0 {
					return fmt.Errorf("expected at least one pod for Elasticsearch %s in namespace %s", esName, namespace)
				}
				if err = k.DeletePod(pods[0]); err != nil {
					return err
				}
				return nil
			}),
		},
	}
}
