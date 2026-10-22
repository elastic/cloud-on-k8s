// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// skipIfBackgroundTasksUnsupported skips the test when the stack version is below 8.16.0,
// the minimum version that supports background task isolation via node.roles, or when no test
// license is available, as spec.backgroundTasks requires an Enterprise license.
func skipIfBackgroundTasksUnsupported(t *testing.T) {
	t.Helper()
	if test.Ctx().TestLicense == "" {
		t.Skip("spec.backgroundTasks requires an Enterprise license, but no test license is configured")
	}
	v := version.MustParse(test.Ctx().ElasticStackVersion)
	if v.LT(kbv1.BackgroundTasksMinVersion) {
		t.Skipf("background task isolation requires Kibana >= %s (got %s)", kbv1.BackgroundTasksMinVersion, v)
	}
}

// TestKibanaBackgroundTasksSplit verifies the steady-state when spec.backgroundTasks is set:
// two Deployments exist, pods carry the correct role labels, and NODE_ROLES env var is injected.
func TestKibanaBackgroundTasksSplit(t *testing.T) {
	skipIfBackgroundTasksUnsupported(t)

	name := "test-kb-bg-split"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithBackgroundTasks(1)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "UI Deployment should exist with role=primary selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimaryValue)
				}),
			},
			{
				Name: "Background tasks Deployment should exist with role=background_tasks selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.BackgroundTasksDeployment(kbBuilder.Kibana.Name), kblabel.RoleBackgroundTasksValue)
				}),
			},
			{
				Name: "UI pods should carry role=primary label",
				Test: test.Eventually(func() error {
					return checkPodsRoleLabel(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RolePrimaryValue, 1)
				}),
			},
			{
				Name: "Background tasks pods should carry role=background_tasks label",
				Test: test.Eventually(func() error {
					return checkPodsRoleLabel(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RoleBackgroundTasksValue, 1)
				}),
			},
			{
				Name: "UI pods should have NODE_ROLES=[\"ui\"]",
				Test: test.Eventually(func() error {
					return checkPodsNodeRolesEnv(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RolePrimaryValue, `["ui"]`)
				}),
			},
			{
				Name: "Background tasks pods should have NODE_ROLES=[\"background_tasks\"]",
				Test: test.Eventually(func() error {
					return checkPodsNodeRolesEnv(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RoleBackgroundTasksValue, `["background_tasks"]`)
				}),
			},
			{
				Name: "Kibana status should report both pools and aggregate them at the top level",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&kbBuilder.Kibana), &kb); err != nil {
						return err
					}
					if err := checkPoolStatus(&kb.Status.Pools.Primary, "primary", kblabel.RolePrimaryValue, 1); err != nil {
						return err
					}
					if err := checkPoolStatus(kb.Status.Pools.BackgroundTasks, "backgroundTasks", kblabel.RoleBackgroundTasksValue, 1); err != nil {
						return err
					}
					// top-level status aggregates both pools: sum of counts/available nodes, AND of healths
					if kb.Status.Count != 2 {
						return fmt.Errorf("expected aggregate count 2, got %d", kb.Status.Count)
					}
					if kb.Status.AvailableNodes != 2 {
						return fmt.Errorf("expected 2 aggregate available nodes, got %d", kb.Status.AvailableNodes)
					}
					if kb.Status.Health != commonv1.GreenHealth {
						return fmt.Errorf("expected green aggregate health, got %s", kb.Status.Health)
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, stepsFn, test.LicenseTestBuilder(esBuilder), kbBuilder).RunSequential(t)
}

// TestKibanaBackgroundTasksEnableDisable verifies that toggling spec.backgroundTasks on/off:
// - enabling: creates the BG Deployment
// - disabling: removes the BG Deployment
func TestKibanaBackgroundTasksEnableDisable(t *testing.T) {
	skipIfBackgroundTasksUnsupported(t)

	name := "test-kb-bg-toggle"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	// Start without background tasks.
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	// Mutation 1: enable background tasks.
	kbWithBG := kbBuilder.
		WithBackgroundTasks(1).
		WithMutatedFrom(&kbBuilder)

	// Mutation 2: disable background tasks (back to original spec).
	kbWithoutBG := kbBuilder.WithMutatedFrom(&kbWithBG)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			// After enabling: both Deployments should exist.
			{
				Name: "After enabling split: UI Deployment should have role=primary selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimaryValue)
				}),
			},
			{
				Name: "After enabling split: BG Deployment should exist",
				Test: test.Eventually(func() error {
					return checkDeploymentExists(k, kbBuilder.Kibana.Namespace, kbv1.BackgroundTasksDeployment(kbBuilder.Kibana.Name))
				}),
			},
			{
				Name: "After enabling split: status.pools should report both pools",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&kbBuilder.Kibana), &kb); err != nil {
						return err
					}
					if err := checkPoolStatus(&kb.Status.Pools.Primary, "primary", kblabel.RolePrimaryValue, 1); err != nil {
						return err
					}
					return checkPoolStatus(kb.Status.Pools.BackgroundTasks, "backgroundTasks", kblabel.RoleBackgroundTasksValue, 1)
				}),
			},
		}
	}

	// After disabling: BG Deployment must be gone.
	disabledStepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "After disabling split: BG Deployment should be deleted",
				Test: test.Eventually(func() error {
					var dep appsv1.Deployment
					err := k.Client.Get(context.Background(), types.NamespacedName{
						Namespace: kbBuilder.Kibana.Namespace,
						Name:      kbv1.BackgroundTasksDeployment(kbBuilder.Kibana.Name),
					}, &dep)
					if apierrors.IsNotFound(err) {
						return nil
					}
					if err != nil {
						return err
					}
					return fmt.Errorf("BG Deployment %s still exists", dep.Name)
				}),
			},
			{
				Name: "After disabling split: single Deployment should be healthy",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimaryValue)
				}),
			},
			{
				Name: "After disabling split: background pool status should be cleared",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&kbBuilder.Kibana), &kb); err != nil {
						return err
					}
					if kb.Status.Pools.BackgroundTasks != nil {
						return fmt.Errorf("status.pools.backgroundTasks should be nil when the split is disabled, got %+v", kb.Status.Pools.BackgroundTasks)
					}
					// the primary pool keeps reporting the single remaining Deployment
					return checkPoolStatus(&kb.Status.Pools.Primary, "primary", kblabel.RolePrimaryValue, 1)
				}),
			},
		}
	}

	test.Sequence(nil, func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			WithSteps(kbWithBG.MutationTestSteps(k)).
			WithSteps(stepsFn(k)).
			WithSteps(kbWithoutBG.MutationTestSteps(k)).
			WithSteps(disabledStepsFn(k))
	}, test.LicenseTestBuilder(esBuilder), kbBuilder).RunSequential(t)
}

// TestKibanaBackgroundTasksRequiresLicense verifies the Enterprise license gate on spec.backgroundTasks when no
// license is installed:
//   - creating a Kibana with background tasks is rejected by the webhook,
//   - enabling background tasks on an existing Kibana is rejected by the webhook,
//   - a Kibana whose background tasks were enabled under a valid license keeps being reconciled after the license
//     is removed, with a warning event.
func TestKibanaBackgroundTasksRequiresLicense(t *testing.T) {
	skipIfBackgroundTasksUnsupported(t)
	if test.Ctx().DeployChaosJob {
		// the chaos job may leave the webhook without a ready endpoint, in which case the API server admits the resource
		t.Skip()
	}

	name := "test-kb-bg-license"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	// start without background tasks and without a license
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)
	// mutation 1: enable background tasks (applied once a license is installed)
	kbWithBG := kbBuilder.
		WithBackgroundTasks(1).
		WithMutatedFrom(&kbBuilder)
	// mutation 2: turn background tasks off again while the license is gone
	kbWithoutBG := kbBuilder.WithMutatedFrom(&kbWithBG)

	// a second Kibana used to exercise the create path; never expected to exist
	rejectedBuilder := kibana.NewBuilder(name + "-create").
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithBackgroundTasks(1)

	nsn := k8s.ExtractNamespacedName(&kbBuilder.Kibana)
	bgDeployment := types.NamespacedName{Namespace: nsn.Namespace, Name: kbv1.BackgroundTasksDeployment(nsn.Name)}

	before := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Remove any Enterprise license secret",
				Test: func(t *testing.T) {
					t.Helper()
					test.DeleteAllEnterpriseLicenseSecrets(t, k)
				},
			},
		}
	}

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Creating a Kibana with background tasks without a license should be rejected",
				Test: func(t *testing.T) {
					t.Helper()
					t.Cleanup(func() {
						_ = k8sclient.IgnoreNotFound(k.Client.Delete(context.Background(), &rejectedBuilder.Kibana))
					})
					var createErr error
					test.Eventually(func() error {
						createErr = k.Client.Create(context.Background(), rejectedBuilder.Kibana.DeepCopy())
						switch {
						case createErr == nil:
							// admitted: the webhook cache may not have seen the license deletion yet, undo and retry
							if err := k8sclient.IgnoreNotFound(k.Client.Delete(context.Background(), &rejectedBuilder.Kibana)); err != nil {
								return err
							}
							return fmt.Errorf("creation of %s was admitted, expected a rejection", rejectedBuilder.Kibana.Name)
						case apierrors.IsInvalid(createErr):
							return nil
						default:
							return createErr
						}
					})(t)
					require.ErrorContains(t, createErr, "spec.backgroundTasks")
					require.ErrorContains(t, createErr, "Enterprise license")
				},
			},
			{
				Name: "Enabling background tasks on an existing Kibana without a license should be rejected",
				Test: func(t *testing.T) {
					t.Helper()
					var updateErr error
					test.Eventually(func() error {
						var kb kbv1.Kibana
						if err := k.Client.Get(context.Background(), nsn, &kb); err != nil {
							return err
						}
						kb.Spec.BackgroundTasks = kbWithBG.Kibana.Spec.BackgroundTasks.DeepCopy()
						updateErr = k.Client.Update(context.Background(), &kb)
						switch {
						case updateErr == nil:
							// admitted: the webhook cache may not have seen the license deletion yet, undo and retry.
							// Leaving background tasks stored would make the later licensed mutation a no-op.
							kb.Spec.BackgroundTasks = nil
							if err := k.Client.Update(context.Background(), &kb); err != nil {
								return err
							}
							return fmt.Errorf("update enabling background tasks on %s was admitted, expected a rejection", nsn.Name)
						case apierrors.IsInvalid(updateErr):
							return nil
						default:
							return updateErr
						}
					})(t)
					require.ErrorContains(t, updateErr, "spec.backgroundTasks")
					require.ErrorContains(t, updateErr, "Enterprise license")
				},
			},
			{
				Name: "Kibana should not have a background tasks Deployment",
				Test: func(t *testing.T) {
					t.Helper()
					var dep appsv1.Deployment
					err := k.Client.Get(context.Background(), bgDeployment, &dep)
					require.True(t, apierrors.IsNotFound(err), "expected no background tasks Deployment, got err=%v", err)
				},
			},
			{
				Name: "Install an Enterprise license secret",
				Test: func(t *testing.T) {
					t.Helper()
					licenseBytes, err := os.ReadFile(test.Ctx().TestLicense)
					require.NoError(t, err)
					test.CreateEnterpriseLicenseSecret(t, k, "eck-license", licenseBytes)
				},
			},
		}.
			// enabling background tasks is admitted once the webhook sees the license; the mutation step retries until then
			WithSteps(kbWithBG.MutationTestSteps(k)).
			WithStep(test.Step{
				Name: "Background tasks Deployment should exist",
				Test: test.Eventually(func() error {
					return checkDeploymentExists(k, bgDeployment.Namespace, bgDeployment.Name)
				}),
			}).
			WithStep(test.Step{
				Name: "Remove the Enterprise license secret",
				Test: func(t *testing.T) {
					t.Helper()
					test.DeleteAllEnterpriseLicenseSecrets(t, k)
				},
			}).
			WithStep(test.Step{
				Name: "Changing a Kibana that keeps background tasks without a license should be rejected",
				Test: func(t *testing.T) {
					t.Helper()
					var updateErr error
					test.Eventually(func() error {
						var kb kbv1.Kibana
						if err := k.Client.Get(context.Background(), nsn, &kb); err != nil {
							return err
						}
						if kb.Annotations == nil {
							kb.Annotations = map[string]string{}
						}
						kb.Annotations["e2e.k8s.elastic.co/unlicensed-change"] = "true"
						updateErr = k.Client.Update(context.Background(), &kb)
						switch {
						case updateErr == nil:
							// admitted: the webhook cache may not have seen the license deletion yet, retry
							return fmt.Errorf("update keeping background tasks on %s was admitted, expected a rejection", nsn.Name)
						case apierrors.IsInvalid(updateErr):
							return nil
						default:
							return updateErr
						}
					})(t)
					require.ErrorContains(t, updateErr, "spec.backgroundTasks")
					require.ErrorContains(t, updateErr, "Enterprise license")
				},
			}).
			WithStep(test.Step{
				Name: "Deleting a background tasks pod should trigger a reconciliation of the stored Kibana",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(test.KibanaPodListOptions(nsn.Namespace, nsn.Name)...)
					if err != nil {
						return err
					}
					for i := range pods {
						if pods[i].Labels[kblabel.RoleLabelName] == kblabel.RoleBackgroundTasksValue {
							return k8sclient.IgnoreNotFound(k.Client.Delete(context.Background(), &pods[i]))
						}
					}
					return fmt.Errorf("no background tasks pod found for %s", nsn.Name)
				}),
			}).
			// the stored resource keeps its background tasks pool: the controller warns and reconciles
			WithStep(test.Step{
				Name: "A license warning event should be emitted for the already enabled background tasks",
				Test: test.Eventually(func() error {
					eventList, err := k.GetEvents(test.EventListOptions(nsn.Namespace, nsn.Name)...)
					if err != nil {
						return err
					}
					for _, evt := range eventList {
						if evt.Type == corev1.EventTypeWarning && evt.Reason == events.EventReasonInvalidLicense {
							return nil
						}
					}
					return fmt.Errorf("event did not fire: %s", events.EventReasonInvalidLicense)
				}),
			}).
			WithStep(test.Step{
				Name: "Background tasks Deployment should still be available without a license",
				Test: test.Eventually(func() error {
					return checkDeploymentExists(k, bgDeployment.Namespace, bgDeployment.Name)
				}),
			}).
			// turning background tasks off is always admitted
			WithSteps(kbWithoutBG.MutationTestSteps(k)).
			WithStep(test.Step{
				Name: "Background tasks Deployment should be deleted after turning background tasks off without a license",
				Test: test.Eventually(func() error {
					var dep appsv1.Deployment
					err := k.Client.Get(context.Background(), bgDeployment, &dep)
					if apierrors.IsNotFound(err) {
						return nil
					}
					if err != nil {
						return err
					}
					return fmt.Errorf("background tasks Deployment %s still exists", dep.Name)
				}),
			})
	}

	test.Sequence(before, stepsFn, esBuilder, kbBuilder).RunSequential(t)
}

// checkPoolStatus asserts that a per-pool status reports the expected replica count, green health
// and a selector scoped to the given role.
func checkPoolStatus(pool *commonv1.DeploymentStatus, poolName, wantRole string, wantCount int32) error {
	if pool == nil {
		return fmt.Errorf("status.pools.%s is nil", poolName)
	}
	if pool.Count != wantCount {
		return fmt.Errorf("status.pools.%s: expected count %d, got %d", poolName, wantCount, pool.Count)
	}
	if pool.AvailableNodes != wantCount {
		return fmt.Errorf("status.pools.%s: expected %d available nodes, got %d", poolName, wantCount, pool.AvailableNodes)
	}
	if pool.Health != commonv1.GreenHealth {
		return fmt.Errorf("status.pools.%s: expected green health, got %s", poolName, pool.Health)
	}
	if !strings.Contains(pool.Selector, fmt.Sprintf("%s=%s", kblabel.RoleLabelName, wantRole)) {
		return fmt.Errorf("status.pools.%s: selector %q does not match role %s", poolName, pool.Selector, wantRole)
	}
	return nil
}

// checkDeploymentSelector asserts that the named Deployment's matchLabels contain the expected role value.
func checkDeploymentSelector(k *test.K8sClient, namespace, name, wantRole string) error {
	var dep appsv1.Deployment
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &dep); err != nil {
		return err
	}
	gotRole, ok := dep.Spec.Selector.MatchLabels[kblabel.RoleLabelName]
	if !ok {
		return fmt.Errorf("Deployment %s selector missing label %s", name, kblabel.RoleLabelName)
	}
	if gotRole != wantRole {
		return fmt.Errorf("Deployment %s selector: expected role=%s, got role=%s", name, wantRole, gotRole)
	}
	return nil
}

// checkDeploymentExists asserts that the named Deployment exists and has at least one available replica.
func checkDeploymentExists(k *test.K8sClient, namespace, name string) error {
	var dep appsv1.Deployment
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, &dep); err != nil {
		return err
	}
	if dep.Status.AvailableReplicas < 1 {
		return fmt.Errorf("Deployment %s has %d available replicas, want >= 1", name, dep.Status.AvailableReplicas)
	}
	return nil
}

// checkPodsRoleLabel lists pods in the given namespace that belong to the Kibana instance and
// carry the specified role label value, then asserts that exactly wantCount such pods are ready.
func checkPodsRoleLabel(k *test.K8sClient, namespace, kbName, wantRole string, wantCount int) error {
	pods, err := k.GetPods(
		k8sclient.InNamespace(namespace),
		k8sclient.MatchingLabels{
			kblabel.KibanaNameLabelName: kbName,
			kblabel.RoleLabelName:       wantRole,
		},
	)
	if err != nil {
		return err
	}

	ready := 0
	for _, p := range pods {
		if k8s.IsPodReady(p) {
			ready++
		}
	}
	if ready != wantCount {
		return fmt.Errorf("role=%s: want %d ready pods, got %d", wantRole, wantCount, ready)
	}
	return nil
}

// checkPodsNodeRolesEnv asserts that all pods with the given role label have the NODE_ROLES env
// var set to wantValue on the kibana container.
func checkPodsNodeRolesEnv(k *test.K8sClient, namespace, kbName, role, wantValue string) error {
	pods, err := k.GetPods(
		k8sclient.InNamespace(namespace),
		k8sclient.MatchingLabels{
			kblabel.KibanaNameLabelName: kbName,
			kblabel.RoleLabelName:       role,
		},
	)
	if err != nil {
		return err
	}
	if len(pods) == 0 {
		return fmt.Errorf("no pods found for role=%s", role)
	}
	for _, pod := range pods {
		if err := checkNodeRolesEnvOnPod(pod, wantValue); err != nil {
			return err
		}
	}
	return nil
}

func checkNodeRolesEnvOnPod(pod corev1.Pod, wantValue string) error {
	for _, c := range pod.Spec.Containers {
		if c.Name != kbv1.KibanaContainerName {
			continue
		}
		for _, env := range c.Env {
			if env.Name == kblabel.NodeRolesEnvVar {
				if env.Value != wantValue {
					return fmt.Errorf("pod %s: NODE_ROLES=%q, want %q", pod.Name, env.Value, wantValue)
				}
				return nil
			}
		}
		return fmt.Errorf("pod %s: NODE_ROLES env var not found on kibana container", pod.Name)
	}
	return fmt.Errorf("pod %s: kibana container not found", pod.Name)
}
