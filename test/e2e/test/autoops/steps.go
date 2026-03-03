// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
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
			Name: "Deploy Cloud Connected API mock",
			Test: test.Eventually(func() error {
				return deployCloudConnectedAPIMock(k)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.AutoOpsAgentPolicy.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "AutoOpsAgentPolicy CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &autoopsv1alpha1.AutoOpsAgentPolicyList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove AutoOpsAgentPolicy if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for AutoOps Agent pods to disappear
				if err := k.CheckPodCount(0, b.ListOptions()...); err != nil {
					return err
				}

				var policy autoopsv1alpha1.AutoOpsAgentPolicy
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy), &policy)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("autoopsagentpolicy %s still exists", k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithStep(test.Step{
			// Wait for all ES clusters matching the policy selector to be reachable before creating the
			// AutoOpsAgentPolicy. The autoops-agent (elastic-otel-collector) initialises its metricbeat
			// receiver immediately on startup and does not retry on a failed initial connection. If ES is
			// not yet accepting connections at that point the receiver stays in a failed state, the
			// OTel health-check endpoint returns HTTP 500, and the pod readiness probe never passes.
			// This step guards against that race between ECK marking ES Ready and the ES HTTP endpoint
			// being truly accessible.
			Name: "ES clusters matching selector should be reachable before creating AutoOpsAgentPolicy",
			Test: test.Eventually(func() error {
				return b.checkMatchingESClustersReachable(k)
			}),
		}).
		WithStep(test.Step{
			Name: "Creating configuration secret should succeed",
			Test: func(t *testing.T) {
				t.Helper()
				if err := k.CreateOrUpdateSecrets(b.ConfigSecret); err != nil {
					require.NoError(t, err)
				}
			},
		}).
		WithStep(test.Step{
			Name: "Creating an AutoOpsAgentPolicy should succeed",
			Test: func(t *testing.T) {
				t.Helper()
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(context.Background(), obj)
					if err != nil && !apierrors.IsAlreadyExists(err) {
						require.NoError(t, err)
					}
				}
			},
		}).
		WithStep(test.Step{
			Name: "AutoOpsAgentPolicy should eventually be created",
			Test: test.Eventually(func() error {
				var createdPolicy autoopsv1alpha1.AutoOpsAgentPolicy
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy), &createdPolicy); err != nil {
					return err
				}
				if b.AutoOpsAgentPolicy.Spec.Version != createdPolicy.Spec.Version {
					return fmt.Errorf("expected version %s but got %s", b.AutoOpsAgentPolicy.Spec.Version, createdPolicy.Spec.Version)
				}
				return nil
			}),
		})
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithStep(test.Step{
			Name: "AutoOpsAgentPolicy status should be ready",
			Test: test.Eventually(func() error {
				var policy autoopsv1alpha1.AutoOpsAgentPolicy
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy), &policy); err != nil {
					return err
				}
				if policy.Status.Phase != autoopsv1alpha1.ReadyPhase {
					return fmt.Errorf("policy not ready, phase: %s", policy.Status.Phase)
				}
				if policy.Status.Resources == 0 {
					return fmt.Errorf("no resources found matching selector")
				}
				if policy.Status.Ready == 0 {
					return fmt.Errorf("no ready resources, ready: %d, errors: %d", policy.Status.Ready, policy.Status.Errors)
				}
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "AutoOps Agent deployments should be ready",
			Test: test.Eventually(func() error {
				return b.forEachAutoOpsDeployment(k, func(deployment appsv1.Deployment) error {
					// Check if deployment is available.
					// Eventually this behavior should fail, as the deployment should be not Ready
					// as the beats receiver failing should never allow the Pod to be in a Ready state.
					// See https://github.com/elastic/beats/issues/47848 for details.
					available := false
					for _, condition := range deployment.Status.Conditions {
						if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
							available = true
							break
						}
					}

					if !available {
						return fmt.Errorf("deployment %s not available yet, replicas: %d/%d ready",
							deployment.Name,
							deployment.Status.ReadyReplicas,
							deployment.Status.Replicas)
					}

					return nil
				})
			}),
		})
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithStep(test.Step{
			Name: "AutoOps Agent pods should be ready",
			Test: test.Eventually(func() error {
				return b.forEachAutoOpsDeployment(k, func(deployment appsv1.Deployment) error {
					// get pods of deployment
					var pods corev1.PodList
					podSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
						MatchLabels: deployment.Spec.Selector.MatchLabels,
					})
					if err != nil {
						return err
					}
					if err := k.Client.List(context.Background(), &pods, k8sclient.InNamespace(b.AutoOpsAgentPolicy.Namespace), k8sclient.MatchingLabelsSelector{Selector: podSelector}); err != nil {
						return err
					}

					if len(pods.Items) == 0 {
						return fmt.Errorf("no pods found for deployment %s", deployment.Name)
					}

					// Check all pods are ready
					for _, pod := range pods.Items {
						if pod.Status.Phase != corev1.PodRunning {
							return fmt.Errorf("pod %s not running, phase: %s", pod.Name, pod.Status.Phase)
						}
						if !k8s.IsPodReady(pod) {
							return fmt.Errorf("pod %s not ready", pod.Name)
						}
					}

					return nil
				})
			}),
		})
}

func (b Builder) forEachAutoOpsDeployment(k *test.K8sClient, fn func(deployment appsv1.Deployment) error) error {
	// Find all Elasticsearch instances that match the resource selector
	var esList esv1.ElasticsearchList
	selector, err := metav1.LabelSelectorAsSelector(&b.AutoOpsAgentPolicy.Spec.ResourceSelector)
	if err != nil {
		return err
	}
	if err := k.Client.List(context.Background(), &esList, &k8sclient.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return err
	}

	isNamespaceAllowed, err := k8s.NamespaceFilterFunc(context.Background(), k.Client, b.AutoOpsAgentPolicy.Spec.NamespaceSelector)
	if err != nil {
		return err
	}

	// Check pods for each ES instance
	for _, es := range esList.Items {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			continue
		}

		deploymentName := autoopsv1alpha1.Deployment(b.AutoOpsAgentPolicy.Name, es)
		var deployment appsv1.Deployment
		err := k.Client.Get(context.Background(), types.NamespacedName{
			Namespace: b.AutoOpsAgentPolicy.Namespace,
			Name:      deploymentName,
		}, &deployment)

		switch {
		// if deployment is not present while it should be filtered out, continue (expected behavior).
		case !isNamespaceAllowed(es.Namespace) && err != nil && apierrors.IsNotFound(err):
			continue

		// if deployment is present while it should be filtered out, return error.
		case !isNamespaceAllowed(es.Namespace) && err == nil:
			return fmt.Errorf("deployment %s should not exist due to namespace selector", deploymentName)

		// on different type of error, fail (no matter the namespace filter).
		case err != nil:
			return err
		}

		if err := fn(deployment); err != nil {
			return err
		}
	}

	return nil
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}.
		WithStep(test.Step{
			Name: "Deleting AutoOpsAgentPolicy should succeed",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "AutoOpsAgentPolicy should be removed",
			Test: test.Eventually(func() error {
				var policy autoopsv1alpha1.AutoOpsAgentPolicy
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy), &policy)
				if err != nil && apierrors.IsNotFound(err) {
					return nil
				}
				if err == nil {
					return fmt.Errorf("autoopsagentpolicy %s is still there", k8s.ExtractNamespacedName(&b.AutoOpsAgentPolicy))
				}
				return err
			}),
		}).
		WithStep(test.Step{
			Name: "Deleting configuration secret should succeed",
			Test: test.Eventually(func() error {
				err := k.DeleteSecrets(b.ConfigSecret)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				return nil
			}),
		}).
		WithStep(test.Step{
			Name: "Deleting Cloud Connected API mock should succeed",
			Test: test.Eventually(func() error {
				return deleteCloudConnectedAPIMock(k)
			}),
		})
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{}
}

// checkMatchingESClustersReachable verifies that every ES cluster that this policy will monitor has
// a reachable and healthy HTTP endpoint. It mirrors the filtering logic used by the operator so that
// only clusters which will actually receive an autoops-agent deployment are checked.
func (b Builder) checkMatchingESClustersReachable(k *test.K8sClient) error {
	selector, err := metav1.LabelSelectorAsSelector(&b.AutoOpsAgentPolicy.Spec.ResourceSelector)
	if err != nil {
		return fmt.Errorf("parsing resource selector: %w", err)
	}

	var esList esv1.ElasticsearchList
	if err := k.Client.List(context.Background(), &esList, &k8sclient.ListOptions{LabelSelector: selector}); err != nil {
		return fmt.Errorf("listing ES clusters: %w", err)
	}

	isNamespaceAllowed, err := k8s.NamespaceFilterFunc(context.Background(), k.Client, b.AutoOpsAgentPolicy.Spec.NamespaceSelector)
	if err != nil {
		return fmt.Errorf("building namespace filter: %w", err)
	}

	for _, es := range esList.Items {
		if !isNamespaceAllowed(es.Namespace) {
			continue
		}
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			return fmt.Errorf("ES cluster %s/%s is not ready yet (phase: %s)", es.Namespace, es.Name, es.Status.Phase)
		}
		esClient, err := elasticsearch.NewElasticsearchClient(es, k)
		if err != nil {
			return fmt.Errorf("creating ES client for %s/%s: %w", es.Namespace, es.Name, err)
		}
		health, err := esClient.GetClusterHealth(context.Background())
		if err != nil {
			return fmt.Errorf("ES cluster %s/%s not reachable: %w", es.Namespace, es.Name, err)
		}
		if health.Status == esv1.ElasticsearchRedHealth {
			return fmt.Errorf("ES cluster %s/%s health is red", es.Namespace, es.Name)
		}
	}
	return nil
}
