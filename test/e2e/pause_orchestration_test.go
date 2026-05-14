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

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	emslabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/maps"
	eprlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry/label"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/epr"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
	ems "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/maps"
)

func TestPauseOrchestration(t *testing.T) {
	namespace := test.Ctx().ManagedNamespace(0)

	initial, phase1, phase2, phase3, phase4, phase6 := pauseOrchestrationBuilders(t)

	pauseOrchestrationSteps := func(k *test.K8sClient) test.StepList {
		// Create with pause orchestration disabled (default)
		steps := test.StepList{}
		for _, b := range initial {
			if !b.SkipTest() {
				steps = steps.WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, false))
			}
		}

		// Phase 1: enable pause-orchestration annotation
		for _, b := range phase1 {
			if !b.SkipTest() {
				steps = steps.
					WithSteps(b.UpgradeTestSteps(k)). // TODO switch back to Mutation?
					// WithSteps(elasticsearch.AnnotatePodsWithBuilderHash(initialBuilder, k)).
					// WithSteps(enabledBuilder.MutationTestSteps(k)).
					WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, b, false))
			}
		}

		// Phase 2: update topology of each application
		for i, b := range phase2 {
			if !b.SkipTest() {
				steps = steps.WithSteps(b.UpgradeTestSteps(k)).
					WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, b, true)).
					// This checks that the topology still matches the previous builder's topology expectation, and
					// assumes each phase's builders were added in the same order.
					WithSteps(test.CheckTestSteps(phase1[i], k))
			}
		}

		// Phase 3: disable pause-orchestration
		for _, b := range phase3 {
			if !b.SkipTest() {
				steps = steps.WithSteps(b.UpgradeTestSteps(k)).
					WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, true)).
					WithSteps(test.CheckTestSteps(b, k)) // Check topology after disabling the annotation
			}
		}

		// Phase 4: re-enable pause-orchestration
		for _, b := range phase4 {
			if !b.SkipTest() {
				steps = steps.WithSteps(b.MutationTestSteps(k)).
					WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, b, false))
			}
		}

		// Phase 5: delete pod
		for _, b := range phase4 { // There are no phase5 builders because we're just deleting a pod; re-use phase4 builders
			if !b.SkipTest() {
				steps = steps.WithStep(deletePod(t, k, namespace, b)).
					WithSteps(test.CheckTestSteps(b, k)).
					WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, b, false))
			}
		}

		// Phase 6: re-disable pause-orchestration
		for _, b := range phase6 {
			if !b.SkipTest() {
				steps = steps.
					WithSteps(b.UpgradeTestSteps(k)).
					WithSteps(test.CheckTestSteps(b, k)).
					WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, b, true))
			}
		}
		return steps
	}

	test.Sequence(nil, pauseOrchestrationSteps, initial...).RunSequential(t)
}

func verifyPauseOrchestrationEnabled(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder, specChangesMade bool) test.Step {
	t.Helper()
	name := builder.ResourceName()
	typ := typeForBuilder(t, name)
	verb := "not"
	if specChangesMade {
		verb = "have been"
	}
	return test.Step{
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
		return verifyDeploymentOrchestrationPaused(t, obj, specChangesExpected)
	}
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

		if orchestrationCondition.Message != common.PausedWithPendingChangesMessage {
			return fmt.Errorf("condition message '%s' is incorrect when spec has changed", orchestrationCondition.Message)
		}
	} else {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			return fmt.Errorf("elasticsearch phase should be %s but was %s", esv1.ElasticsearchReadyPhase, es.Status.Phase)
		}

		if orchestrationCondition.Message != common.PausedNoChangesMessage {
			return fmt.Errorf("condition message '%s' is incorrect when spec has not changed", orchestrationCondition.Message)
		}
	}
	return nil
}

func verifyDeploymentOrchestrationPaused(t *testing.T, obj k8sclient.Object, specChangesMade bool) error {
	t.Helper()
	var status commonv1.DeploymentStatus
	switch obj.(type) {
	case *kbv1.Kibana:
		kb, ok := obj.(*kbv1.Kibana)
		require.Truef(t, ok, "expected Kibana resource but got %T", obj)
		status = kb.Status.DeploymentStatus
	case *apmv1.ApmServer:
		apm, ok := obj.(*apmv1.ApmServer)
		require.Truef(t, ok, "expected APM Server resource but got %T", obj)
		status = apm.Status.DeploymentStatus
	case *eprv1alpha1.PackageRegistry:
		e, ok := obj.(*eprv1alpha1.PackageRegistry)
		require.Truef(t, ok, "expected Package Registry resource but got %T", obj)
		status = e.Status.DeploymentStatus
	case *emsv1alpha1.ElasticMapsServer:
		e, ok := obj.(*emsv1alpha1.ElasticMapsServer)
		require.Truef(t, ok, "expected Elastic Maps Server resource but got %T", obj)
		status = e.Status.DeploymentStatus
	default:
		return fmt.Errorf("unexpected Deployment type %T", obj)
	}

	orchestrationPausedIndex := status.Conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex < 0 {
		return fmt.Errorf("%s condition does not exist on %s resource", commonv1.OrchestrationPaused, obj.GetName())
	}

	orchestrationCondition := status.Conditions[orchestrationPausedIndex]
	if orchestrationCondition.Status == corev1.ConditionFalse {
		return fmt.Errorf("condition %s should be true on %s", commonv1.OrchestrationPaused, obj.GetName())
	}

	if specChangesMade {
		if orchestrationCondition.Message != common.PausedWithPendingChangesMessage {
			return fmt.Errorf("condition message '%s' is incorrect when spec has changed", orchestrationCondition.Message)
		}
	} else {
		if orchestrationCondition.Message != common.PausedNoChangesMessage {
			return fmt.Errorf("condition message '%s' is incorrect when spec has not changed", orchestrationCondition.Message)
		}
	}
	return nil
}

func verifyPauseOrchestrationDisabled(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder, previouslyPaused bool) test.StepList {
	t.Helper()
	name := builder.ResourceName()
	typ := typeForBuilder(t, name)
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
		return verifyDeploymentOrchestrationUnpaused(t, obj, previouslyPaused)
	}
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

func verifyDeploymentOrchestrationUnpaused(t *testing.T, obj k8sclient.Object, previouslyPaused bool) error {
	t.Helper()
	var status commonv1.DeploymentStatus
	switch obj.(type) {
	case *kbv1.Kibana:
		kb, ok := obj.(*kbv1.Kibana)
		require.Truef(t, ok, "expected Kibana resource but got %T", obj)
		status = kb.Status.DeploymentStatus
	case *apmv1.ApmServer:
		apm, ok := obj.(*apmv1.ApmServer)
		require.Truef(t, ok, "expected APM Server resource but got %T", obj)
		status = apm.Status.DeploymentStatus
	case *eprv1alpha1.PackageRegistry:
		e, ok := obj.(*eprv1alpha1.PackageRegistry)
		require.Truef(t, ok, "expected Package Registry resource but got %T", obj)
		status = e.Status.DeploymentStatus
	default:
		return fmt.Errorf("unexpected Deployment type %T", obj)
	}

	orchestrationPausedIndex := status.Conditions.Index(commonv1.OrchestrationPaused)
	if !previouslyPaused && orchestrationPausedIndex >= 0 {
		return fmt.Errorf("%s condition should not exist on the %s resource", commonv1.OrchestrationPaused, commonv1.OrchestrationPaused)
	}

	if orchestrationPausedIndex >= 0 && status.Conditions[orchestrationPausedIndex].Status == corev1.ConditionTrue {
		return fmt.Errorf("condition %s should be false", commonv1.OrchestrationPaused)
	}
	return nil
}

func deletePod(t *testing.T, k *test.K8sClient, namespace string, builder test.Builder) test.Step {
	t.Helper()
	name := builder.ResourceName()
	return test.Step{
		Name: fmt.Sprintf("A new pod becomes ready when a pod is deleted for %s/%s", namespace, builder.ResourceName()),
		Test: test.Eventually(func() error {
			typ := typeForBuilder(t, name)
			pods, err := k.GetPods(listOptionsForType(t, namespace, typ)...)
			if err != nil {
				return err
			}
			if len(pods) == 0 {
				return fmt.Errorf("expected at least one pod for %s/%s", namespace, name)
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
	case kblabel.Type:
		return &kbv1.Kibana{}
	case apmv1.Type:
		return &apmv1.ApmServer{}
	case eprlabel.Type:
		return &eprv1alpha1.PackageRegistry{}
	case emslabels.Type:
		return &emsv1alpha1.ElasticMapsServer{}
	default:
		t.Fatalf("unknown type: %s", typ)
	}
	return nil
}

func typeForBuilder(t *testing.T, fullName string) string {
	t.Helper()
	for _, typ := range []string{label.Type, kblabel.Type, apmv1.Type, eprlabel.Type} {
		if strings.Contains(fullName, typ) {
			return typ
		}
	}
	t.Fatalf("no known type for resource: %s", fullName)
	return ""
}

func pauseOrchestrationBuilders(t *testing.T) (
	initial []test.Builder,
	phase1 []test.Builder,
	phase2 []test.Builder,
	phase3 []test.Builder,
	phase4 []test.Builder,
	phase6 []test.Builder) {
	t.Helper()
	initial = make([]test.Builder, 0)
	// Start with pause-orchestration disabled (default)
	// Elasticsearch
	esInitial := elasticsearch.NewBuilder(testName(label.Type)).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	// TODO fill in the stateless apps
	esRef := commonv1.ObjectSelector{Namespace: esInitial.Elasticsearch.Namespace, Name: esInitial.Elasticsearch.Name}
	// EPR
	eprInitial := epr.NewBuilder(testName(eprlabel.Type)).
		WithNodeCount(1).
		WithRestrictedSecurityContext()
	// Kibana
	kbInitial := kibana.NewBuilder(testName(kblabel.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithPackageRegistryRef(eprInitial.Ref()).
		WithRestrictedSecurityContext().
		WithAPMIntegration()
	kbRef := commonv1.ObjectSelector{Namespace: kbInitial.Kibana.Namespace, Name: kbInitial.Kibana.Name}
	// APM
	apmInitial := apmserver.NewBuilder(testName(apmv1.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithKibanaRef(kbRef).
		WithRestrictedSecurityContext()
	// EnterpriseSearch - unsupported in 9.x
	// entInitial := enterprisesearch.NewBuilder(testName(entv1.Type)).
	// 	WithElasticsearchRef(esRef).
	// 	WithNodeCount(1).
	// 	WithRestrictedSecurityContext()
	// EMS
	emsInitial := ems.NewBuilder(testName(emslabels.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithRestrictedSecurityContext()
	// TODO implement non-Deployment paths of the following 2
	// Beats
	// Agent
	initial = append(initial, esInitial, eprInitial, kbInitial, apmInitial, emsInitial)

	// Phase 1: transition to pause-orchestration: true
	phase1 = make([]test.Builder, 0)
	esEnabled := esInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esInitial)
	eprEnabled := eprInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&eprInitial)
	kbEnabled := kbInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&kbInitial)
	apmEnabled := apmInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&apmInitial)
	// entEnabled := entInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&entInitial)
	emsEnabled := emsInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&emsInitial)
	phase1 = append(phase1, esEnabled, eprEnabled, kbEnabled, apmEnabled, emsEnabled)

	// Phase 2: update topology of each application - add 1 to each
	phase2 = make([]test.Builder, 0)
	esUpdated := esEnabled.DeepCopy().WithESCoordinatingNodes(1, elasticsearch.DefaultResources).WithMutatedFrom(&esEnabled)
	eprUpdated := eprEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&eprEnabled)
	kbUpdated := kbEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&kbEnabled)
	apmUpdated := apmEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&apmEnabled)
	// entUpdated := entEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&entEnabled)
	emsUpdated := emsEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&emsEnabled)
	phase2 = append(phase2, esUpdated, eprUpdated, kbUpdated, apmUpdated, emsUpdated)

	// Phase 3: transition back to disabled
	phase3 = make([]test.Builder, 0)
	esDisabled := esUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esUpdated)
	eprDisabled := eprUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&eprUpdated)
	kbDisabled := kbUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&kbUpdated)
	apmDisabled := apmUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&apmUpdated)
	// entDisabled := entUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&entUpdated)
	emsDisabled := emsUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&emsUpdated)
	phase3 = append(phase3, esDisabled, eprDisabled, kbDisabled, apmDisabled, emsDisabled)

	// Phase 4: transition back to enabled again
	phase4 = make([]test.Builder, 0)
	esReenabled := esDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esDisabled)
	eprReenabled := eprDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&eprDisabled)
	kbReenabled := kbDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&kbDisabled)
	apmReenabled := apmDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&apmDisabled)
	// entReenabled := entDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&entDisabled)
	emsReenabled := emsDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&emsDisabled)
	phase4 = append(phase4, esReenabled, eprReenabled, kbReenabled, apmReenabled, emsReenabled)

	// Phase 5: pod deletion (no builders)

	// Phase 6: re-disable the annotation
	phase6 = make([]test.Builder, 0)
	esRedisabled := esReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esReenabled)
	eprRedisabled := eprReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&eprReenabled)
	kbRedisabled := kbReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&kbReenabled)
	apmRedisabled := apmReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&apmReenabled)
	// entRedisabled := entReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&entReenabled)
	emsRedisabled := emsReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&emsReenabled)
	phase6 = append(phase6, esRedisabled, eprRedisabled, kbRedisabled, apmRedisabled, emsRedisabled)

	return initial, phase1, phase2, phase3, phase4, phase6
}

func testName(typ string) string {
	return fmt.Sprintf("test-pause-%s", typ)
}
