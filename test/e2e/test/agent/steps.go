// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/generation"
)

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: test.Eventually(func() error {
				pods := corev1.PodList{}
				return k.Client.List(context.Background(), &pods)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.Agent.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Agent CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &agentv1alpha1.AgentList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove Agent if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for Agent pods to disappear
				if err := k.CheckPodCount(0, test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...); err != nil {
					return err
				}

				// it may take some extra time for Agent to be fully deleted
				var agent agentv1alpha1.Agent
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("agent %s is still there", k8s.ExtractNamespacedName(&b.Agent))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	// increase the test timeout for autopilot clusters because autoscaling k8s nodes time can increase the time
	// before the elasticsearch container is ready
	timeout := test.Ctx().TestTimeout
	if test.Ctx().AutopilotCluster {
		timeout *= 2
	}
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				// This is to improve test stability see https://github.com/elastic/cloud-on-k8s/issues/7172
				Name: "Wait for Elasticsearch to be green",
				Test: test.UntilSuccess(func() error {
					for _, ref := range b.Agent.Spec.ElasticsearchRefs {
						var es esv1.Elasticsearch
						if err := k.Client.Get(context.Background(), ref.WithDefaultNamespace(b.Agent.Namespace).NamespacedName(), &es); err != nil {
							return err
						}
						if es.Status.Health != esv1.ElasticsearchGreenHealth {
							return fmt.Errorf("health is not green but %s", es.Status.Health)
						}
					}
					return nil
				}, timeout),
				Skip: func() bool {
					return len(b.Agent.Spec.ElasticsearchRefs) == 0
				},
			},
			test.Step{
				Name: "Creating an Agent should succeed",
				Test: func(t *testing.T) {
					t.Helper()
					for _, obj := range b.RuntimeObjects() {
						err := k.Client.Create(context.Background(), obj)
						require.NoError(t, err)
					}
				},
			},
			test.Step{
				Name: "Agent should be created",
				Test: func(t *testing.T) {
					t.Helper()
					var createdAgent agentv1alpha1.Agent
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &createdAgent)
					require.NoError(t, err)
					require.Equal(t, b.Agent.Spec.Version, createdAgent.Spec.Version)
				},
			},
		})
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Agent status should be updated",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}
				// don't check association statuses that may vary across tests
				agent.Status.ElasticsearchAssociationsStatus = nil
				agent.Status.KibanaAssociationStatus = ""
				agent.Status.FleetServerAssociationStatus = ""
				agent.Status.ObservedGeneration = 0

				expected := agentv1alpha1.AgentStatus{
					Version: b.Agent.Spec.Version,
					Health:  "green",
				}

				switch {
				case b.Agent.Spec.Deployment != nil:
					expectedReplicas := pointer.Int32OrDefault(b.Agent.Spec.Deployment.Replicas, int32(1))
					expected.ExpectedNodes = expectedReplicas
					expected.AvailableNodes = expectedReplicas
				case b.Agent.Spec.StatefulSet != nil:
					expectedReplicas := pointer.Int32OrDefault(b.Agent.Spec.StatefulSet.Replicas, int32(1))
					expected.ExpectedNodes = expectedReplicas
					expected.AvailableNodes = expectedReplicas
				default:
					// don't check the replicas count for daemonsets
					agent.Status.ExpectedNodes = 0
					agent.Status.AvailableNodes = 0
				}
				if !reflect.DeepEqual(agent.Status, expected) {
					return fmt.Errorf("expected status %+v but got %+v", expected, agent.Status)
				}
				return nil
			}),
		},
	}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	// health
	return test.StepList{
		test.Step{
			Name: "Agent health should be green",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}

				if agent.Status.Health != agentv1alpha1.AgentGreenHealth {
					return fmt.Errorf("agent %s health is %s", agent.Name, agent.Status.Health)
				}

				return nil
			}),
		},
		test.Step{
			Name: "ES data should pass validations",
			Test: test.Eventually(func() error {
				for i, validation := range b.Validations {
					expectedOutputName := b.ValidationsOutputs[i]
					var esNsName types.NamespacedName
					for _, output := range b.Agent.Spec.ElasticsearchRefs {
						if output.OutputName == expectedOutputName ||
							output.OutputName == "" && len(b.Agent.Spec.ElasticsearchRefs) == 1 {
							esNsName = output.WithDefaultNamespace(b.Agent.Namespace).NamespacedName()
							break
						}
					}

					if esNsName.Namespace == "" && esNsName.Name == "" {
						return fmt.Errorf("agent %s doesn't have output %s", b.Agent.Name, expectedOutputName)
					}

					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), esNsName, &es); err != nil {
						return err
					}

					esClient, err := elasticsearch.NewElasticsearchClient(es, k)
					if err != nil {
						return err
					}

					if err := validation(esClient); err != nil {
						return err
					}
				}

				return nil
			}),
		},
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	//nolint:thelper
	return test.StepList{
		{
			Name: "Applying the Agent mutation should succeed",
			Test: func(t *testing.T) {
				var agent agentv1alpha1.Agent
				require.NoError(t, k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent))
				agent.Spec = b.Agent.Spec
				require.NoError(t, k.Client.Update(context.Background(), &agent))
			},
		}}
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	//nolint:thelper
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "The resources should not be there anymore",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					objCopy := k8s.DeepCopyObject(obj)
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(obj), objCopy)
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
					}
					return errors.Wrap(err, "expected 404 not found API error here")
				}
				return nil
			}),
		},
		{
			Name: "Agent pods should be eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...)
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation int64

	isMutated := b.MutatedFrom != nil

	return test.StepList{
		generation.RetrieveGenerationsStep(&b.Agent, k, &agentGenerationBeforeMutation, &agentObservedGenerationBeforeMutation),
	}.WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.Agent, k, isMutated, agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation))
}
