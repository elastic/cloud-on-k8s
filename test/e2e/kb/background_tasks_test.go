// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build kb || e2e

package kb

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	kblabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// skipIfBackgroundTasksUnsupported skips the test when the stack version is below 8.16.0,
// the minimum version that supports background task isolation via node.roles.
func skipIfBackgroundTasksUnsupported(t *testing.T) {
	t.Helper()
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
				Name: "UI Deployment should exist with role=prime selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimeValue)
				}),
			},
			{
				Name: "Background tasks Deployment should exist with role=background_tasks selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.BackgroundTasksDeployment(kbBuilder.Kibana.Name), kblabel.RoleBackgroundTasksValue)
				}),
			},
			{
				Name: "UI pods should carry role=prime label",
				Test: test.Eventually(func() error {
					return checkPodsRoleLabel(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RolePrimeValue, 1)
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
					return checkPodsNodeRolesEnv(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RolePrimeValue, `["ui"]`)
				}),
			},
			{
				Name: "Background tasks pods should have NODE_ROLES=[\"background_tasks\"]",
				Test: test.Eventually(func() error {
					return checkPodsNodeRolesEnv(k, kbBuilder.Kibana.Namespace, kbBuilder.Kibana.Name, kblabel.RoleBackgroundTasksValue, `["background_tasks"]`)
				}),
			},
			{
				Name: "KibanaStatus.BackgroundTasks should be populated",
				Test: test.Eventually(func() error {
					var kb kbv1.Kibana
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&kbBuilder.Kibana), &kb); err != nil {
						return err
					}
					if kb.Status.BackgroundTasks == nil {
						return fmt.Errorf("KibanaStatus.BackgroundTasks is nil")
					}
					if kb.Status.BackgroundTasks.AvailableNodes != 1 {
						return fmt.Errorf("expected 1 available background tasks node, got %d", kb.Status.BackgroundTasks.AvailableNodes)
					}
					return nil
				}),
			},
		}
	}

	test.Sequence(nil, stepsFn, esBuilder, kbBuilder).RunSequential(t)
}

// TestKibanaBackgroundTasksEnableDisable verifies that toggling spec.backgroundTasks on/off:
// - enabling: creates the BG Deployment
// - disabling: removes the BG Deployment
func TestKibanaBackgroundTasksEnableDisable(t *testing.T) {
	skipIfBackgroundTasksUnsupported(t)

	name := "test-kb-bg-toggle"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

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
				Name: "After enabling split: UI Deployment should have role=prime selector",
				Test: test.Eventually(func() error {
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimeValue)
				}),
			},
			{
				Name: "After enabling split: BG Deployment should exist",
				Test: test.Eventually(func() error {
					return checkDeploymentExists(k, kbBuilder.Kibana.Namespace, kbv1.BackgroundTasksDeployment(kbBuilder.Kibana.Name))
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
					return checkDeploymentSelector(k, kbBuilder.Kibana.Namespace, kbv1.Deployment(kbBuilder.Kibana.Name), kblabel.RolePrimeValue)
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
	}, esBuilder, kbBuilder).RunSequential(t)
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
