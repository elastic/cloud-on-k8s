package agent

import (
	"context"
	"testing"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
)

func TestAgentObservedGenerationIncrement(t *testing.T) {
	version := test.Ctx().ElasticStackVersion

	name := "test-agent-update-labels"
	esBuilder := elasticsearch.NewBuilder(name).
		WithVersion(version).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	agentBuilder := agent.NewBuilder(name + "-ea").
		WithVersion(version).
		WithRoles(agent.PSPClusterRoleName).
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default"))

	mutatedAgentBuilder := agentBuilder.
		WithPodLabel("new", "label")

	k := test.NewK8sClientOrFatal()

	var initialGeneration, initialObservedGeneration int64
	var eventualAgent agentv1alpha1.Agent

	test.StepList{}.
		WithSteps(esBuilder.InitTestSteps(k)).WithSteps(esBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(esBuilder, k)).
		WithSteps(agentBuilder.InitTestSteps(k)).WithSteps(agentBuilder.CreationTestSteps(k)).
		WithSteps(test.CheckTestSteps(agentBuilder, k)).
		WithStep(test.Step{
			Name: "Get Agent initial generation",
			Test: test.Eventually(func() error {
				var createdAgent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&agentBuilder.Agent), &createdAgent); err != nil {
					return err
				}
				initialGeneration = createdAgent.Generation
				initialObservedGeneration = createdAgent.Status.ObservedGeneration
				return nil
			}),
		}).
		WithSteps(mutatedAgentBuilder.UpgradeTestSteps(k)).
		WithSteps(test.CheckTestSteps(agentBuilder, k)).
		WithSteps(test.StepList{
			{
				Name: "Get mutated Agent",
				Test: test.Eventually(func() error {
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&agentBuilder.Agent), &eventualAgent); err != nil {
						return err
					}
					return nil
				}),
			},
			{
				Name: "Agent.Generation should have been incremented; Agent.Status.ObservedGeneration should have been incremented; Agent.Status.ObsservedGeneration should equal Agent.Generation",
				Test: func(t *testing.T) {
					if eventualAgent.Generation < initialGeneration {
						t.Errorf("Generation of Agent should have been incremented, current: %d, previous: %d", eventualAgent.Generation, initialGeneration)
					}
					if eventualAgent.Status.ObservedGeneration < initialObservedGeneration {
						t.Errorf("Status.ObservedGeneration of Agent should have been incremented, current: %d, previous: %d", eventualAgent.Status.ObservedGeneration, initialObservedGeneration)
					}

					if eventualAgent.Status.ObservedGeneration != eventualAgent.Generation {
						t.Errorf("Status.ObservedGeneration of Agent should equal current generation, current: %d, observedGeneration: %d", eventualAgent.Generation, eventualAgent.Status.ObservedGeneration)
					}
				},
			},
		}).RunSequential(t)
}
