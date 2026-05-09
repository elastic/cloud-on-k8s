// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

func TestPauseOrchestration(t *testing.T) {
	namespace := test.Ctx().ManagedNamespace(0)

	initial, phase1, phase2, phase3, phase4, phase6 := pauseOrchestrationBuilders(t)

	pauseOrchestrationSteps := func(k *test.K8sClient) test.StepList {
		// Create with pause orchestration disabled (default)
		steps := test.StepList{}
		for _, b := range initial {
			steps = steps.WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, 1, false))
		}

		// Phase 1: enable pause-orchestration annotation
		for _, b := range phase1 {
			steps = steps.
				WithSteps(b.MutationTestSteps(k)).
				WithSteps(verifyPauseOrchestrationEnabled(t, k, namespace, b, 1, false))
		}

		// Phase 2: update stack version
		for _, b := range phase2 {
			steps = steps.WithSteps(b.UpgradeTestSteps(k)).
				WithSteps(verifyPauseOrchestrationEnabled(t, k, namespace, b, 1, true))
		}

		// Phase 3: disable pause-orchestration
		for _, b := range phase3 {
			steps = steps.WithSteps(b.UpgradeTestSteps(k)).
				WithSteps(test.CheckTestSteps(b, k)).
				WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, 3, true))
		}

		// Phase 4: re-enable pause-orchestration
		for _, b := range phase4 {
			steps = steps.WithSteps(b.MutationTestSteps(k)).
				WithSteps(verifyPauseOrchestrationEnabled(t, k, namespace, b, 3, false))
		}

		// Phase 5: delete pod
		for _, b := range phase4 { // There are no phase5 builders because we're just deleting a pod; re-use phase4 builders
			steps = steps.WithStep(deletePod(t, k, namespace, b)).
				WithSteps(test.CheckTestSteps(b, k)).
				WithSteps(verifyPauseOrchestrationEnabled(t, k, namespace, b, 3, false))
		}

		// Phase 6: re-disable pause-orchestration
		for _, b := range phase6 {
			steps = steps.
				WithSteps(b.UpgradeTestSteps(k)).
				WithSteps(test.CheckTestSteps(b, k)).
				WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, 3, true))
		}
		return steps
	}

	test.Sequence(nil, pauseOrchestrationSteps, initial...).RunSequential(t)
}

func verifyPauseOrchestrationEnabled(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder, expectedNodeCount int, specChangesMade bool) test.StepList {
	t.Helper()
	name := builder.ResourceName()
	typ := typeForBuilder(t, builder)
	verb := "not "
	if specChangesMade {
		verb = "have been"
	}
	return test.StepList{
		{
			Name: fmt.Sprintf("Verify pause-orchestration annotation is true when spec changes %s made for %s/%s", verb, namespace, name),
			Test: test.Eventually(func() error {
				obj := objectForType(t, typ)
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}, obj); err != nil {
					return err
				}

				return verifyStackComponentPaused(t, typ, obj, specChangesMade)
			}),
		},
		{
			Name: fmt.Sprintf("Verify expected number of pods are running for %s/%s", namespace, name),
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(listOptionsForType(t, namespace, typ)...)
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

func verifyStackComponentPaused(t *testing.T, typ string, obj k8sclient.Object, specChangesExpected bool) error {
	t.Helper()
	if !common.IsOrchestrationPaused(obj) {
		return fmt.Errorf("annotation %s should be set to true", common.PauseOrchestrationAnnotation)
	}

	switch typ {
	case label.Type:
		return verifyElasticsearchOrchestrationPaused(t, obj, specChangesExpected)
	default:
		t.Fatalf("unknown type %s", typ)
	}
	return nil
}

func verifyElasticsearchOrchestrationPaused(t *testing.T, obj k8sclient.Object, specChangesMade bool) error {
	t.Helper()
	es, ok := obj.(*esv1.Elasticsearch)
	require.Truef(t, ok, "expected Elasticsearch resource but got %T", obj)
	orchestrationPausedIndex := es.Status.Conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex < 0 {
		return fmt.Errorf("%s condition does not exist on Elasticsearch resource", commonv1.OrchestrationPaused)
	}

	orchestrationCondition := es.Status.Conditions[orchestrationPausedIndex]
	if orchestrationCondition.Status == corev1.ConditionFalse {
		return fmt.Errorf("condition %s should be true", commonv1.OrchestrationPaused)
	}

	if specChangesMade {
		if es.Status.Phase != esv1.ElasticsearchApplyingChangesPhase {
			return fmt.Errorf("elasticsearch phase should be %s but was %s", esv1.ElasticsearchApplyingChangesPhase, es.Status.Phase)
		}

		if orchestrationCondition.Message != "Orchestration paused via annotation; spec changes are pending and will be applied on resume" {
			return fmt.Errorf("condition message '%s' is incorrect when spec has changed", orchestrationCondition.Message)
		}
	} else {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			return fmt.Errorf("elasticsearch phase should be %s but was %s", esv1.ElasticsearchReadyPhase, es.Status.Phase)
		}

		if orchestrationCondition.Message != "Orchestration paused via annotation; no pending spec changes detected" {
			return fmt.Errorf("condition message '%s' is incorrect when spec has not changed", orchestrationCondition.Message)
		}
	}
	return nil
}

func verifyPauseOrchestrationDisabled(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder, expectedNodeCount int, previouslyPaused bool) test.StepList {
	t.Helper()
	name := builder.ResourceName()
	typ := typeForBuilder(t, builder)
	return test.StepList{
		{
			Name: fmt.Sprintf("Verify pause-orchestration annotation is set to false for %s/%s", namespace, name),
			Test: test.Eventually(func() error {
				obj := objectForType(t, typ)
				if err := k.Client.Get(context.Background(), types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}, obj); err != nil {
					return err
				}

				return verifyStackComponentUnpaused(t, typ, obj, previouslyPaused)
			}),
		},
		{
			Name: fmt.Sprintf("Verify all expected pods are running for %s/%s", namespace, name),
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(listOptionsForType(t, namespace, typ)...)
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

func verifyStackComponentUnpaused(t *testing.T, typ string, obj k8sclient.Object, previouslyPaused bool) error {
	t.Helper()
	if common.IsOrchestrationPaused(obj) {
		return fmt.Errorf("annotation %s should be set to false", common.PauseOrchestrationAnnotation)
	}

	switch typ {
	case label.Type:
		return verifyElasticsearchOrchestrationUnpaused(t, obj, previouslyPaused)
	default:
		t.Fatalf("unknown type %s", typ)
	}
	return nil
}

func verifyElasticsearchOrchestrationUnpaused(t *testing.T, obj k8sclient.Object, previouslyPaused bool) error {
	t.Helper()
	es, ok := obj.(*esv1.Elasticsearch)
	require.Truef(t, ok, "expected Elasticsearch resource but got %T", obj)
	if es.Status.Phase != esv1.ElasticsearchReadyPhase {
		return fmt.Errorf("elasticsearch phase should be %s", esv1.ElasticsearchReadyPhase)
	}

	orchestrationPausedIndex := es.Status.Conditions.Index(commonv1.OrchestrationPaused)
	if !previouslyPaused && orchestrationPausedIndex >= 0 {
		return fmt.Errorf("%s condition should not exist on Elasticsearch resource", commonv1.OrchestrationPaused)
	}

	if orchestrationPausedIndex >= 0 && es.Status.Conditions[orchestrationPausedIndex].Status == corev1.ConditionTrue {
		return fmt.Errorf("condition %s should be false", commonv1.OrchestrationPaused)
	}
	return nil
}

func deletePod(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder) test.Step {
	t.Helper()
	return test.Step{
		Name: "A new pod becomes ready when a pod is deleted",
		Test: test.Eventually(func() error {
			typ := typeForBuilder(t, builder)
			pods, err := k.GetPods(listOptionsForType(t, namespace, typ)...)
			if err != nil {
				return err
			}
			if len(pods) == 0 {
				return fmt.Errorf("expected at least one pod for %s/%s", namespace, builder.ResourceName())
			}
			if err = k.DeletePod(pods[0]); err != nil {
				return err
			}
			return nil
		}),
	}
}

func listOptionsForType(t *testing.T, namespace string, typ string) []k8sclient.ListOption {
	t.Helper()
	ns := k8sclient.InNamespace(namespace)
	matchLabels := k8sclient.MatchingLabels(map[string]string{
		commonv1.TypeLabelName: typ,
	})
	return []k8sclient.ListOption{ns, matchLabels}
}

func objectForType(t *testing.T, typ string) k8sclient.Object {
	t.Helper()
	switch typ {
	case label.Type:
		return &esv1.Elasticsearch{}
	default:
		t.Fatalf("unknown type: %s", typ)
	}
	return nil
}

func typeForBuilder(t *testing.T, builder test.Builder) string {
	t.Helper()
	namePieces := strings.Split(builder.ResourceName(), "-")
	require.Greaterf(t, len(namePieces), 1, "expected more than one name piece for %s", builder.ResourceName())
	return namePieces[len(namePieces)-2] // the last piece is the short uuid added by test infrastructure
}

func pauseOrchestrationBuilders(t *testing.T) (
	initial []test.Builder,
	phase1 []test.Builder,
	phase2 []test.Builder,
	phase3 []test.Builder,
	phase4 []test.Builder,
	phase6 []test.Builder) {
	t.Helper()
	testName := "test-pause"
	initial = make([]test.Builder, 0)
	// Start with pause-orchestration disabled (default)
	// Elasticsearch
	esInitial := elasticsearch.NewBuilder(fmt.Sprintf("%s-%s", testName, label.Type)).WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	initial = append(initial, esInitial)
	// TODO fill in the stateless apps
	// Kibana
	// APM
	// EnterpriseSearch
	// Beats
	// Agent
	phase1 = make([]test.Builder, 0)
	// Phase 1: transition to pause-orchestration: true
	esEnabled := esInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esInitial)
	phase1 = append(phase1, esEnabled)
	// Phase 2: update Elasticsearch node spec
	phase2 = make([]test.Builder, 0)
	esUpdated := esEnabled.DeepCopy().WithESMasterDataNodes(3, elasticsearch.DefaultResources).WithMutatedFrom(&esEnabled)
	phase2 = append(phase2, esUpdated)
	// Phase 3: transition back to disabled
	phase3 = make([]test.Builder, 0)
	esDisabled := esUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esUpdated)
	phase3 = append(phase3, esDisabled)
	// Phase 4: transition back to enabled again
	phase4 = make([]test.Builder, 0)
	esReenabled := esDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esDisabled)
	phase4 = append(phase4, esReenabled)
	// Phase 5: pod deletion (no builders)
	// Phase 6: re-disable the annotation
	phase6 = make([]test.Builder, 0)
	esRedisabled := esReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esReenabled)
	phase6 = append(phase6, esRedisabled)

	return initial, phase1, phase2, phase3, phase4, phase6
}
