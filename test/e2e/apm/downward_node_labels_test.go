// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
)

// TestDownwardNodeLabels verifies that when DownwardNodeLabelsAnnotation is set on an
// ApmServer, the operator injects a wait-for-annotations init container using its own image,
// that the init container completes successfully, the requested node label is copied as a pod
// annotation, and that the annotation value is accessible inside the main container via an
// env var derived from the downward API.
func TestDownwardNodeLabels(t *testing.T) {
	if test.Ctx().OperatorImage == "" {
		t.Skip("operator image not set in test context, skipping downward-node-labels test")
	}

	// topology.kubernetes.io/zone is present on every node in any conformant cluster and is
	// allowed by the default exposed-node-labels policy.
	const (
		nodeLabel  = "topology.kubernetes.io/zone"
		envVarName = "NODE_ZONE"
	)

	name := "test-dnl"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)

	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithoutIntegrationCheck().
		WithAnnotation(commonv1.DownwardNodeLabelsAnnotation, nodeLabel).
		WithContainerEnvVars(apmv1.ApmServerContainerName, corev1.EnvVar{
			Name: envVarName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fmt.Sprintf("metadata.annotations['%s']", nodeLabel),
				},
			},
		})

	checkSteps := func(k *test.K8sClient) test.StepList {
		listOpts := test.ApmServerPodListOptions(apmBuilder.ApmServer.Namespace, apmBuilder.ApmServer.Name)
		return test.StepList{
			{
				Name: "wait-for-annotations init container should run with operator image, complete successfully, and copy node label as pod annotation",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(listOpts...)
					if err != nil {
						return err
					}
					if len(pods) == 0 {
						return fmt.Errorf("no pods found")
					}
					for _, pod := range pods {
						ic, found := findInitContainerByName(pod, commonnodelabels.WaitForAnnotationsContainerName)
						if !found {
							return fmt.Errorf("wait-for-annotations init container not found in pod %s", pod.Name)
						}
						if ic.Image != test.Ctx().OperatorImage {
							return fmt.Errorf("pod %s: expected init container image %q, got %q",
								pod.Name, test.Ctx().OperatorImage, ic.Image)
						}
						var icStatus *corev1.ContainerStatus
						for i := range pod.Status.InitContainerStatuses {
							if pod.Status.InitContainerStatuses[i].Name == commonnodelabels.WaitForAnnotationsContainerName {
								icStatus = &pod.Status.InitContainerStatuses[i]
								break
							}
						}
						if icStatus == nil || icStatus.State.Terminated == nil || icStatus.State.Terminated.ExitCode != 0 {
							return fmt.Errorf("pod %s: init container has not completed successfully: %v",
								pod.Name, icStatus)
						}
						if _, ok := pod.Annotations[nodeLabel]; !ok {
							return fmt.Errorf("expected annotation %q not found on pod %s", nodeLabel, pod.Name)
						}
					}
					return nil
				}),
			},
			{
				Name: "NODE_ZONE env var inside the main container should match the node's zone label",
				Test: test.Eventually(func() error {
					pods, err := k.GetPods(listOpts...)
					if err != nil {
						return err
					}
					if len(pods) == 0 {
						return fmt.Errorf("no pods found")
					}
					return test.OnAllPods(pods, func(pod corev1.Pod) error {
						var node corev1.Node
						if err := k.Client.Get(t.Context(), client.ObjectKey{Name: pod.Spec.NodeName}, &node); err != nil {
							return fmt.Errorf("failed to get node %s: %w", pod.Spec.NodeName, err)
						}
						wantZone, ok := node.Labels[nodeLabel]
						if !ok {
							return fmt.Errorf("node %s does not have label %s", pod.Spec.NodeName, nodeLabel)
						}

						nsn := k8s.ExtractNamespacedName(&pod)
						stdout, _, err := k.ExecInContainer(nsn, apmv1.ApmServerContainerName,
							[]string{"printenv", envVarName})
						if err != nil {
							return fmt.Errorf("exec into pod %s failed: %w", pod.Name, err)
						}
						if gotZone := strings.TrimRight(stdout, "\n"); gotZone != wantZone {
							return fmt.Errorf("pod %s: %s=%q, want %q (node %s)",
								pod.Name, envVarName, gotZone, wantZone, pod.Spec.NodeName)
						}
						return nil
					})
				}),
			},
		}
	}

	test.Sequence(nil, checkSteps, esBuilder, apmBuilder).RunSequential(t)
}

func findInitContainerByName(pod corev1.Pod, name string) (corev1.Container, bool) {
	for _, ic := range pod.Spec.InitContainers {
		if ic.Name == name {
			return ic, true
		}
	}
	return corev1.Container{}, false
}
