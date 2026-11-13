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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	beatv1b1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/maps/v1alpha1"
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	agentlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	apmlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoops"
	beatcommon "github.com/elastic/cloud-on-k8s/v3/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/beat/filebeat"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	entlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/enterprisesearch"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	lslabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/logstash/labels"
	emslabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/maps"
	eprlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry/label"
	e2e_agent "github.com/elastic/cloud-on-k8s/v3/test/e2e/agent"
	beattests "github.com/elastic/cloud-on-k8s/v3/test/e2e/beat"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	testautoops "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/autoops"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/beat"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/epr"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/logstash"
	ems "github.com/elastic/cloud-on-k8s/v3/test/e2e/test/maps"
)

// All PauseOrchestration tests include Elasticsearch and Kibana

func TestPauseOrchestration_APM(t *testing.T) {
	testSequenceForTypes(t, apmlabel.Type)
}

func TestPauseOrchestration_PackageRegistryAndMaps(t *testing.T) {
	testSequenceForTypes(t, eprlabel.Type, emslabels.Type)
}

func TestPauseOrchestration_EnterpriseSearch(t *testing.T) {
	testSequenceForTypes(t, entlabel.Type)
}

func TestPauseOrchestration_Agent(t *testing.T) {
	testSequenceForTypes(t, agentlabel.TypeLabelValue)
}

func TestPauseOrchestration_Beat(t *testing.T) {
	testSequenceForTypes(t, beatcommon.TypeLabelValue)
}

func TestPauseOrchestration_Logstash(t *testing.T) {
	testSequenceForTypes(t, lslabels.TypeLabelValue)
}

// TestPauseOrchestration_AutoOps tests the pause-orchestration annotation for AutoOpsAgentPolicy.
// It uses a dedicated sequence that does NOT pause the underlying Elasticsearch cluster, avoiding
// the ES readiness gate that would otherwise prevent AutoOps from computing pending Deployment changes.
func TestPauseOrchestration_AutoOps(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(version.SupportedAutoOpsAgentEnterpriseVersions.Min) {
		t.Skipf("Skipping test: version %s below minimum %s",
			test.Ctx().ElasticStackVersion, version.SupportedAutoOpsAgentEnterpriseVersions.Min)
	}

	namespace := test.Ctx().ManagedNamespace(0)

	esInitial := elasticsearch.NewBuilder(testName(label.Type)).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext().
		WithLabel("autoops", "enabled")
	esWithLicense := test.LicenseTestBuilder(esInitial)

	mockURL := testautoops.CloudConnectedAPIMockURL()

	aoInitial := testautoops.NewBuilder(testName(autoops.TypeLabelValue)).
		WithNamespace(esInitial.Namespace()).
		WithNamespaceSelector(metav1.LabelSelector{
			MatchLabels: map[string]string{"kubernetes.io/metadata.name": esInitial.Namespace()},
		}).
		WithCloudConnectedAPIURL(mockURL).
		WithAutoOpsOTelURL(mockURL)

	// Phase 1: pause, no spec changes yet
	aoEnabled := aoInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&aoInitial)

	// Phase 2: update resources while paused (values differ from the 400Mi/200m defaults so the template hash changes)
	aoUpdated := aoEnabled.DeepCopy().WithResources(corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("500Mi"),
			corev1.ResourceCPU:    resource.MustParse("300m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("500Mi"),
			corev1.ResourceCPU:    resource.MustParse("300m"),
		},
	}).WithMutatedFrom(&aoEnabled)

	// Phase 3: resume
	aoDisabled := aoUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&aoUpdated)

	// Phase 4: re-pause (no new changes since phase 3)
	aoReenabled := aoDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&aoDisabled)

	// Phase 6: re-disable
	aoRedisabled := aoReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&aoReenabled)

	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			// Initial: verify annotation absent, policy ready
			WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, aoInitial, false)).
			// Phase 1: pause with no pending changes
			WithSteps(aoEnabled.UpgradeTestSteps(k)).
			WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, aoEnabled, false)).
			// Phase 2: change resources while paused; verify change is held
			WithSteps(aoUpdated.UpgradeTestSteps(k)).
			WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, aoUpdated, true)).
			WithSteps(test.CheckTestSteps(aoEnabled, k)).
			// Phase 3: resume; verify resources applied
			WithSteps(aoDisabled.UpgradeTestSteps(k)).
			WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, aoDisabled, true)).
			WithSteps(test.CheckTestSteps(aoDisabled, k)).
			// Phase 4: re-pause
			WithSteps(aoReenabled.MutationTestSteps(k)).
			WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, aoReenabled, false)).
			// Phase 5: delete pod while paused; verify pod recovers
			WithStep(deletePod(t, k, namespace, aoReenabled)).
			WithSteps(test.CheckTestSteps(aoReenabled, k)).
			WithStep(verifyPauseOrchestrationEnabled(t, k, namespace, aoReenabled, false)).
			// Phase 6: re-disable
			WithSteps(aoRedisabled.UpgradeTestSteps(k)).
			WithSteps(test.CheckTestSteps(aoRedisabled, k)).
			WithSteps(verifyPauseOrchestrationDisabled(t, k, namespace, aoRedisabled, true))
	}, esWithLicense, aoInitial).RunSequential(t)
}

func testSequenceForTypes(t *testing.T, typ ...string) {
	t.Helper()
	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	namespace := test.Ctx().ManagedNamespace(0)

	initial, phase1, phase2, phase3, phase4, phase6 := pauseOrchestrationBuilders(t, typ...)

	pauseOrchestrationSteps := createTestStepsForBuilders(t, namespace, initial, phase1, phase2, phase3, phase4, phase6)

	test.Sequence(nil, pauseOrchestrationSteps, initial...).RunSequential(t)
}

func createTestStepsForBuilders(t *testing.T, namespace string, initial, phase1, phase2, phase3, phase4, phase6 []test.Builder) func(k *test.K8sClient) test.StepList {
	t.Helper()
	return func(k *test.K8sClient) test.StepList {
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
					WithSteps(b.UpgradeTestSteps(k)).
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
		return verifyApplicationOrchestrationPaused(t, obj, specChangesExpected)
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

func verifyApplicationOrchestrationPaused(t *testing.T, obj k8sclient.Object, specChangesMade bool) error {
	t.Helper()
	conditions := conditionsForObject(t, obj)
	orchestrationPausedIndex := conditions.Index(commonv1.OrchestrationPaused)
	if orchestrationPausedIndex < 0 {
		return fmt.Errorf("%s condition does not exist on %s resource", commonv1.OrchestrationPaused, obj.GetName())
	}

	orchestrationCondition := conditions[orchestrationPausedIndex]
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
		return verifyApplicationOrchestrationUnpaused(t, obj, previouslyPaused)
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

func verifyApplicationOrchestrationUnpaused(t *testing.T, obj k8sclient.Object, previouslyPaused bool) error {
	t.Helper()
	conditions := conditionsForObject(t, obj)
	orchestrationPausedIndex := conditions.Index(commonv1.OrchestrationPaused)
	if !previouslyPaused && orchestrationPausedIndex >= 0 {
		return fmt.Errorf("%s condition should not exist on the %s resource", commonv1.OrchestrationPaused, commonv1.OrchestrationPaused)
	}

	if orchestrationPausedIndex >= 0 && conditions[orchestrationPausedIndex].Status == corev1.ConditionTrue {
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

func conditionsForObject(t *testing.T, obj k8sclient.Object) commonv1.Conditions {
	t.Helper()
	objectWithConditions, ok := obj.(common.ObjectWithConditions)
	require.Truef(t, ok, "%T does not implement the ObjectWithConditions interface", obj)
	return objectWithConditions.Conditions()
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
	case apmlabel.Type:
		return &apmv1.ApmServer{}
	case eprlabel.Type:
		return &eprv1alpha1.PackageRegistry{}
	case emslabels.Type:
		return &emsv1alpha1.ElasticMapsServer{}
	case entlabel.Type:
		return &entv1.EnterpriseSearch{}
	case agentlabel.TypeLabelValue:
		return &agentv1alpha1.Agent{}
	case beatcommon.TypeLabelValue:
		return &beatv1b1.Beat{}
	case lslabels.TypeLabelValue:
		return &logstashv1alpha1.Logstash{}
	case autoops.TypeLabelValue:
		return &autoopsv1alpha1.AutoOpsAgentPolicy{}
	default:
		t.Fatalf("unknown type: %s", typ)
	}
	return nil
}

func typeForBuilder(t *testing.T, fullName string) string {
	t.Helper()
	for _, typ := range []string{label.Type, kblabel.Type, apmlabel.Type, eprlabel.Type, emslabels.Type, entlabel.Type, autoops.TypeLabelValue, agentlabel.TypeLabelValue, beatcommon.TypeLabelValue, lslabels.TypeLabelValue} {
		if strings.Contains(fullName, typ) {
			return typ
		}
	}
	t.Fatalf("no known type for resource: %s", fullName)
	return ""
}

type phaseBuilder struct {
	testTypes map[string]struct{}
	phase     []test.Builder
}

func newPhaseBuilder(testTypes map[string]struct{}) *phaseBuilder {
	return &phaseBuilder{
		testTypes: testTypes,
		phase:     make([]test.Builder, 0),
	}
}

func (p *phaseBuilder) AddBuilder(testBuilder test.Builder) {
	p.phase = append(p.phase, testBuilder)
}

func (p *phaseBuilder) MaybeAddBuilder(typ string, subject test.Builder) {
	if _, ok := p.testTypes[typ]; ok {
		p.phase = append(p.phase, subject)
	}
}

func pauseOrchestrationBuilders(t *testing.T, optionalTypes ...string) (
	initial []test.Builder,
	phase1 []test.Builder,
	phase2 []test.Builder,
	phase3 []test.Builder,
	phase4 []test.Builder,
	phase6 []test.Builder,
) {
	t.Helper()

	testTypes := make(map[string]struct{})
	for _, typ := range optionalTypes {
		testTypes[typ] = struct{}{}
	}

	// Start with pause-orchestration disabled (default)
	initialBuilder := newPhaseBuilder(testTypes)
	// Elasticsearch - ALWAYS created
	esInitial := elasticsearch.NewBuilder(testName(label.Type)).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	esWithLicense := test.LicenseTestBuilder(esInitial)
	esRef := commonv1.ObjectSelector{Namespace: esInitial.Elasticsearch.Namespace, Name: esInitial.Elasticsearch.Name}
	initialBuilder.AddBuilder(esWithLicense)

	// EPR
	eprInitial := epr.NewBuilder(testName(eprlabel.Type)).
		WithNodeCount(1).
		WithRestrictedSecurityContext()
	initialBuilder.MaybeAddBuilder(eprlabel.Type, eprInitial)

	// Kibana - ALWAYS added
	kbInitial := kibana.NewBuilder(testName(kblabel.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithRestrictedSecurityContext()
	if _, testEPR := testTypes[eprlabel.Type]; testEPR {
		kbInitial = kbInitial.WithPackageRegistryRef(eprInitial.Ref())
	}
	if _, testAPM := testTypes[apmlabel.Type]; testAPM {
		kbInitial = kbInitial.WithAPMIntegration()
	}
	kbRef := commonv1.ObjectSelector{Namespace: kbInitial.Kibana.Namespace, Name: kbInitial.Kibana.Name}
	initialBuilder.AddBuilder(kbInitial)

	// APM
	apmInitial := apmserver.NewBuilder(testName(apmlabel.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithKibanaRef(kbRef).
		WithRestrictedSecurityContext()
	initialBuilder.MaybeAddBuilder(apmlabel.Type, apmInitial)

	// EMS
	emsInitial := ems.NewBuilder(testName(emslabels.Type)).
		WithNodeCount(1).
		WithElasticsearchRef(esRef).
		WithRestrictedSecurityContext()
	initialBuilder.MaybeAddBuilder(emslabels.Type, emsInitial)

	// Beats
	beatInitial := beat.NewBuilder(testName(beatcommon.TypeLabelValue)).
		WithRoles(beat.AutodiscoverClusterRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithType(filebeat.Type).
		WithElasticsearchRef(esRef).
		WithKibanaRef(kbRef)
	fileBeatConfig := beattests.E2EFilebeatConfig
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	// Stack versions 8.0.X to 8.9.X do not support fingerprint identity type
	// Versions 7.17.X and 8.10.X and above support fingerprint identity type
	if !beattests.SupportsFingerprintIdentity(stackVersion) {
		fileBeatConfig = beattests.E2EFilebeatConfigPRE810
	}
	beatInitial = beat.ApplyYamls(t, beatInitial, fileBeatConfig, beattests.E2EFilebeatPodTemplate)
	initialBuilder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatInitial)

	// Agent
	agentInitial := agent.NewBuilder(testName(agentlabel.TypeLabelValue)).
		WithElasticsearchRefs(agent.ToOutput(esRef, "default")).
		WithOpenShiftRoles(test.UseSCCRole).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default"))
	agentInitial = agent.ApplyYamls(t, agentInitial, e2e_agent.E2EAgentSystemIntegrationConfig, e2e_agent.E2EAgentSystemIntegrationPodTemplate).MoreResourcesForIssue4730()
	initialBuilder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentInitial)

	// EnterpriseSearch - unsupported in 9+
	entInitial := enterprisesearch.NewBuilder(testName(entlabel.Type)).
		WithElasticsearchRef(esRef).
		WithNodeCount(1).
		WithRestrictedSecurityContext()
	entSearchEnabled := !entInitial.SkipTest()
	if entSearchEnabled {
		initialBuilder.MaybeAddBuilder(entlabel.Type, entInitial)
	}

	// Logstash
	lsInitial := logstash.NewBuilder(testName(lslabels.TypeLabelValue)).
		WithNodeCount(1).
		WithElasticsearchRefs(logstashv1alpha1.ElasticsearchCluster{
			ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esRef},
			ClusterName:           "monitoring",
		})
	initialBuilder.MaybeAddBuilder(lslabels.TypeLabelValue, lsInitial)

	initial = initialBuilder.phase

	// Phase 1: transition to pause-orchestration: true
	phase1Builder := newPhaseBuilder(testTypes)
	esEnabled := esInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esInitial)
	phase1Builder.AddBuilder(esEnabled)
	eprEnabled := eprInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&eprInitial)
	phase1Builder.MaybeAddBuilder(eprlabel.Type, eprEnabled)
	kbEnabled := kbInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&kbInitial)
	phase1Builder.AddBuilder(kbEnabled)
	apmEnabled := apmInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&apmInitial)
	phase1Builder.MaybeAddBuilder(apmlabel.Type, apmEnabled)
	emsEnabled := emsInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&emsInitial)
	phase1Builder.MaybeAddBuilder(emslabels.Type, emsEnabled)
	beatEnabled := beatInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&beatInitial)
	phase1Builder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatEnabled)
	agentEnabled := agentInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&agentInitial)
	phase1Builder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentEnabled)
	entEnabled := entInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&entInitial)
	if entSearchEnabled {
		phase1Builder.MaybeAddBuilder(entlabel.Type, entEnabled)
	}
	lsEnabled := lsInitial.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&lsInitial)
	phase1Builder.MaybeAddBuilder(lslabels.TypeLabelValue, lsEnabled)
	phase1 = phase1Builder.phase

	// Phase 2: update topology of each application
	phase2Builder := newPhaseBuilder(testTypes)
	esUpdated := esEnabled.DeepCopy().WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources).
		WithMutatedFrom(&esEnabled)
	phase2Builder.AddBuilder(esUpdated)
	eprUpdated := eprEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&eprEnabled)
	phase2Builder.MaybeAddBuilder(eprlabel.Type, eprUpdated)
	kbUpdated := kbEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&kbEnabled)
	phase2Builder.AddBuilder(kbUpdated)
	apmUpdated := apmEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&apmEnabled)
	phase2Builder.MaybeAddBuilder(apmlabel.Type, apmUpdated)
	emsUpdated := emsEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&emsEnabled)
	phase2Builder.MaybeAddBuilder(emslabels.Type, emsUpdated)
	beatUpdated := beatEnabled.DeepCopy().WithResources(corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("550Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("550Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}).WithMutatedFrom(&beatEnabled)
	phase2Builder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatUpdated)
	agentUpdated := agentEnabled.DeepCopy().WithResources(corev1.ResourceRequirements{
		Limits: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("800Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
		Requests: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceMemory: resource.MustParse("800Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}).WithMutatedFrom(&agentEnabled)
	phase2Builder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentUpdated)
	entUpdated := entEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&entEnabled)
	if entSearchEnabled {
		phase2Builder.MaybeAddBuilder(entlabel.Type, entUpdated)
	}
	lsUpdated := lsEnabled.DeepCopy().WithNodeCount(2).WithMutatedFrom(&lsEnabled)
	phase2Builder.MaybeAddBuilder(lslabels.TypeLabelValue, lsUpdated)
	phase2 = phase2Builder.phase

	// Phase 3: transition back to disabled
	phase3Builder := newPhaseBuilder(testTypes)
	esDisabled := esUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esUpdated)
	phase3Builder.AddBuilder(esDisabled)
	eprDisabled := eprUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&eprUpdated)
	phase3Builder.MaybeAddBuilder(eprlabel.Type, eprDisabled)
	kbDisabled := kbUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&kbUpdated)
	phase3Builder.AddBuilder(kbDisabled)
	apmDisabled := apmUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&apmUpdated)
	phase3Builder.MaybeAddBuilder(apmlabel.Type, apmDisabled)
	emsDisabled := emsUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&emsUpdated)
	phase3Builder.MaybeAddBuilder(emslabels.Type, emsDisabled)
	beatDisabled := beatUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&beatUpdated)
	phase3Builder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatDisabled)
	agentDisabled := agentUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&agentUpdated)
	phase3Builder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentDisabled)
	entDisabled := entUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&entUpdated)
	if entSearchEnabled {
		phase3Builder.MaybeAddBuilder(entlabel.Type, entDisabled)
	}
	lsDisabled := lsUpdated.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&lsUpdated)
	phase3Builder.MaybeAddBuilder(lslabels.TypeLabelValue, lsDisabled)
	phase3 = phase3Builder.phase

	// Phase 4: transition back to enabled again
	phase4Builder := newPhaseBuilder(testTypes)
	esReenabled := esDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&esDisabled)
	phase4Builder.AddBuilder(esReenabled)
	eprReenabled := eprDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&eprDisabled)
	phase4Builder.MaybeAddBuilder(eprlabel.Type, eprReenabled)
	kbReenabled := kbDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&kbDisabled)
	phase4Builder.AddBuilder(kbReenabled)
	apmReenabled := apmDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&apmDisabled)
	phase4Builder.MaybeAddBuilder(apmlabel.Type, apmReenabled)
	emsReenabled := emsDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&emsDisabled)
	phase4Builder.MaybeAddBuilder(emslabels.Type, emsReenabled)
	beatReenabled := beatDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&beatDisabled)
	phase4Builder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatReenabled)
	agentReenabled := agentDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&agentDisabled)
	phase4Builder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentReenabled)
	entReenabled := entDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&entDisabled)
	if entSearchEnabled {
		phase4Builder.MaybeAddBuilder(entlabel.Type, entReenabled)
	}
	lsReenabled := lsDisabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "true").WithMutatedFrom(&lsDisabled)
	phase4Builder.MaybeAddBuilder(lslabels.TypeLabelValue, lsReenabled)
	phase4 = phase4Builder.phase

	// Phase 5: pod deletion (no builders)

	// Phase 6: re-disable the annotation
	phase6Builder := newPhaseBuilder(testTypes)
	esRedisabled := esReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&esReenabled)
	phase6Builder.AddBuilder(esRedisabled)
	eprRedisabled := eprReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&eprReenabled)
	phase6Builder.MaybeAddBuilder(eprlabel.Type, eprRedisabled)
	kbRedisabled := kbReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&kbReenabled)
	phase6Builder.AddBuilder(kbRedisabled)
	apmRedisabled := apmReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&apmReenabled)
	phase6Builder.MaybeAddBuilder(apmlabel.Type, apmRedisabled)
	emsRedisabled := emsReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&emsReenabled)
	phase6Builder.MaybeAddBuilder(emslabels.Type, emsRedisabled)
	beatRedisabled := beatReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&beatReenabled)
	phase6Builder.MaybeAddBuilder(beatcommon.TypeLabelValue, beatRedisabled)
	agentRedisabled := agentReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&agentReenabled)
	phase6Builder.MaybeAddBuilder(agentlabel.TypeLabelValue, agentRedisabled)
	entRedisabled := entReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&entReenabled)
	if entSearchEnabled {
		phase6Builder.MaybeAddBuilder(entlabel.Type, entRedisabled)
	}
	lsRedisabled := lsReenabled.DeepCopy().WithAnnotation(common.PauseOrchestrationAnnotation, "false").WithMutatedFrom(&lsReenabled)
	phase6Builder.MaybeAddBuilder(lslabels.TypeLabelValue, lsRedisabled)
	phase6 = phase6Builder.phase

	require.Truef(t,
		len(initial) == len(phase1) &&
			len(phase1) == len(phase2) &&
			len(phase2) == len(phase3) &&
			len(phase3) == len(phase4) &&
			len(phase4) == len(phase6),
		"All phases should have the same number of builders, but they have...\n"+
			"Initial: %d\n"+
			"Phase 1: %d\n"+
			"Phase 2: %d\n"+
			"Phase 3: %d\n"+
			"Phase 4: %d\n"+
			"Phase 6: %d", len(initial), len(phase1), len(phase2), len(phase3), len(phase4), len(phase6))

	return initial, phase1, phase2, phase3, phase4, phase6
}

func testName(typ string) string {
	return fmt.Sprintf("test-pause-%s", typ)
}
