// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch/autoscaling"
)

// TestAutoscaling ensures that the operator is compatible with the autoscaling Elasticsearch API.
// The purpose of this test is only to assess that there is no regression at the API level. It only relies on the
// fixed decider to generate scaling events, other deciders, like storage deciders or ML deciders are not exercised.
// Note that only the node count and the memory limit of the deployed Pods are validated for now, see https://github.com/elastic/cloud-on-k8s/issues/4411.
func TestAutoscaling(t *testing.T) {
	// Autoscaling is only available for stateful Elasticsearch
	test.SkipIfStateless(t, "autoscaling requires stateful Elasticsearch")

	// only execute this test if we have a test license to work with
	if test.Ctx().TestLicense == "" {
		t.SkipNow()
	}

	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	// Autoscaling API is supported since 7.11.0
	if !stackVersion.GTE(version.MustParse("7.11.0")) {
		t.SkipNow()
	}

	k8sClient := test.NewK8sClientOrFatal()
	// This autoscaling test requires a storage class which supports volume expansion.
	storageClass, err := getResizeableStorageClass(k8sClient.Client)
	require.NoError(t, err)
	if storageClass == "" {
		t.Skip("No storage class allowing volume expansion found. Skipping the test.")
	}

	// The test sequence involves 2 tiers:
	// * A data tier with 2 initial nodes.
	// * A ML tier with no node initially started.
	name := "test-autoscaling"
	initialPVC := newPVC("1Gi", storageClass)
	ns1 := test.Ctx().ManagedNamespace(0)
	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(ns1).
		// Create a dedicated master node
		WithNodeSet(newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{}, initialPVC)).
		// Add a data tier, node count is initially set to 0, it will be updated by the autoscaling controller.
		WithNodeSet(newNodeSet("data-ingest", []string{"data", "ingest"}, 0, corev1.ResourceList{}, initialPVC)).
		// Add a ml tier, node count is initially set to 0, it will be updated by the autoscaling controller.
		WithNodeSet(newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC)).
		WithRestrictedSecurityContext().
		WithExpectedNodeSets(
			// master has no autoscaling policy — NodeSet.Resources is not managed by the autoscaler and stays empty.
			newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{}, initialPVC),
			// Autoscaling controller should eventually update the data node count to its min. value.
			newNodeSet("data-ingest", []string{"data", "ingest"}, 2, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, newPVC("10Gi", storageClass)),
			// ML node count should still be 0.
			newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
		)
	autoscalingBuilder := autoscaling.NewAutoscalingBuilder(t, k8s.ExtractNamespacedName(&esBuilder.Elasticsearch)).
		WithPolicy("data-ingest", []string{"data", "ingest"}, v1alpha1.AutoscalingResources{
			CPURange:       &v1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
			MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("4Gi")},
			StorageRange:   &v1alpha1.QuantityRange{Min: resource.MustParse("10Gi"), Max: resource.MustParse("20Gi")},
			NodeCountRange: v1alpha1.CountRange{Min: 2, Max: 4},
		}).
		WithPolicy("ml", []string{"ml"}, v1alpha1.AutoscalingResources{
			CPURange:       &v1alpha1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
			MemoryRange:    &v1alpha1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("4Gi")},
			StorageRange:   &v1alpha1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("1Gi")},
			NodeCountRange: v1alpha1.CountRange{Min: 0, Max: 1},
		})

	// Use the fixed decider to trigger a scale up of the data tier up to its max memory limit and 3 nodes.
	esaScaleUpStorageBuilder := autoscalingBuilder.DeepCopy().WithFixedDecider("data-ingest", map[string]string{"storage": "19gb", "nodes": "3"})
	expectedDataPVC := newPVC("20Gi", storageClass)
	esScaleUpStorageBuilder := esBuilder.DeepCopy().WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	// scaleUpML uses the fixed decider to trigger the creation of a ML node.
	esaScaleUpML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "1"})
	esScaleUpML := esBuilder.DeepCopy().WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 1, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, initialPVC),
	)

	// esaScaleUpLimitedML uses the fixed decider to request the creation of more nodes than allowed.
	esaScaleUpLimitedML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "2"})

	// scaleDownML use the fixed decider to trigger the scale down, and thus the deletion, of the ML node previously created.
	esaScaleDownML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "0gb", "nodes": "0"})
	esScaleDownML := esBuilder.DeepCopy().WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	autoscalerWithLicense := test.LicenseTestBuilder(autoscalingBuilder)

	autoscalingCapacityTest := autoscaling.NewAutoscalingCapacityTest(esBuilder.Elasticsearch, k8sClient)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			WithStep(autoscalingCapacityTest).
			// Autoscaler should be eventually online and healthy
			WithStep(autoscalingBuilder.NewAutoscalingStatusTestBuilder(k8sClient).
				ShouldBeActive().ShouldBeHealthy().ShouldBeOnline().ToStep(),
			).
			WithStep(checkNodeSetResourcesStep(k8sClient, &esBuilder)).
			WithStep(checkAutoscaledPodsResourcesStep(k8sClient, &esBuilder, "data-ingest", "ml")).

			// Scale vertically and horizontally to add some storage capacity
			WithSteps(esaScaleUpStorageBuilder.UpgradeTestSteps(k)).
			WithSteps(esScaleUpStorageBuilder.CheckK8sTestSteps(k)).
			WithSteps(esScaleUpStorageBuilder.CheckStackTestSteps(k)).
			WithStep(checkNodeSetResourcesStep(k8sClient, &esScaleUpStorageBuilder)).
			WithStep(checkAutoscaledPodsResourcesStep(k8sClient, &esScaleUpStorageBuilder, "data-ingest", "ml")).
			WithStep(autoscalingCapacityTest).

			// Scale vertically and horizontally to add some ml capacity
			WithSteps(esaScaleUpML.UpgradeTestSteps(k)).
			WithSteps(esScaleUpML.CheckK8sTestSteps(k)).
			WithSteps(esScaleUpML.CheckStackTestSteps(k)).
			WithStep(checkNodeSetResourcesStep(k8sClient, &esScaleUpML)).
			WithStep(checkAutoscaledPodsResourcesStep(k8sClient, &esScaleUpML, "data-ingest", "ml")).
			WithStep(autoscalingCapacityTest).

			// Scale horizontally up to the limit capacity
			WithSteps(esaScaleUpLimitedML.UpgradeTestSteps(k)).
			// Autoscaler should be limited
			WithStep(autoscalingBuilder.NewAutoscalingStatusTestBuilder(k8sClient).
				ShouldBeActive().ShouldBeHealthy().ShouldBeOnline().ShouldBeLimited().ToStep(),
			).
			WithStep(checkNodeSetResourcesStep(k8sClient, &esScaleUpML)).
			WithStep(checkAutoscaledPodsResourcesStep(k8sClient, &esScaleUpML, "data-ingest", "ml")).

			// Scale ML tier back to 0 node
			WithSteps(esaScaleDownML.UpgradeTestSteps(k)).
			WithSteps(esScaleDownML.CheckK8sTestSteps(k)).
			WithSteps(esScaleDownML.CheckStackTestSteps(k)).
			WithStep(checkNodeSetResourcesStep(k8sClient, &esScaleDownML)).
			WithStep(checkAutoscaledPodsResourcesStep(k8sClient, &esScaleDownML, "data-ingest", "ml")).
			WithStep(autoscalingCapacityTest).
			// Autoscaler should no longer be limited
			WithStep(autoscalingBuilder.NewAutoscalingStatusTestBuilder(k8sClient).
				ShouldBeActive().ShouldBeHealthy().ShouldBeOnline().ToStep(),
			)
	}

	test.Sequence(nil, stepsFn, autoscalerWithLicense, esBuilder).RunSequential(t)
}

// -- Test helpers

// newPVC returns a new volume claim for the Elasticsearch data volume
func newPVC(storageQuantity, storageClass string) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: volume.ElasticsearchDataVolumeName,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageQuantity),
				},
			},
			StorageClassName: &storageClass,
		},
	}
}

// newNodeSet returns a NodeSet with the provided properties.
func newNodeSet(name string, roles []string, count int32, limits corev1.ResourceList, pvc corev1.PersistentVolumeClaim) esv1.NodeSet {
	return esv1.NodeSet{
		Name: name,
		Config: &commonv1.Config{
			Data: map[string]any{esv1.NodeRoles: roles},
		},
		Count:                count,
		Resources:            newNodeSetResources(limits),
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{pvc},
		PodTemplate: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: esv1.ElasticsearchContainerName,
						Resources: corev1.ResourceRequirements{
							Limits: limits,
						},
					},
				},
			},
		},
	}
}

// newNodeSetResources maps CPU and memory limits to NodeSet shorthand resources.
// This mirrors autoscaling reconciliation, which now writes recommendations into
// spec.nodeSets[].resources instead of only mutating the PodTemplate container resources.
func newNodeSetResources(limits corev1.ResourceList) commonv1.Resources {
	resources := commonv1.Resources{
		Requests: commonv1.ResourceAllocations{},
		Limits:   commonv1.ResourceAllocations{},
	}

	if memory, exists := limits[corev1.ResourceMemory]; exists {
		memoryReq := memory
		memoryLimit := memory
		resources.Requests.Memory = &memoryReq
		resources.Limits.Memory = &memoryLimit
	}
	if cpu, exists := limits[corev1.ResourceCPU]; exists {
		cpuReq := cpu
		cpuLimit := cpu
		resources.Requests.CPU = &cpuReq
		resources.Limits.CPU = &cpuLimit
	}
	return resources
}

// checkNodeSetResourcesStep verifies that Elasticsearch NodeSet shorthand resources are in sync
// with the resources expected by the test scenario.
func checkNodeSetResourcesStep(k8sClient *test.K8sClient, expectedBuilder *elasticsearch.Builder) test.Step {
	return test.Step{
		Name: "Elasticsearch NodeSet resources should match expected values",
		Test: test.Eventually(func() error {
			expectedES := expectedBuilder.GetExpectedElasticsearch()

			var actualES esv1.Elasticsearch
			if err := k8sClient.Client.Get(context.Background(), k8s.ExtractNamespacedName(&expectedES), &actualES); err != nil {
				return err
			}

			for _, expectedNodeSet := range expectedES.Spec.NodeSets {
				var actualNodeSet *esv1.NodeSet
				for i := range actualES.Spec.NodeSets {
					if actualES.Spec.NodeSets[i].Name == expectedNodeSet.Name {
						actualNodeSet = &actualES.Spec.NodeSets[i]
						break
					}
				}

				if actualNodeSet == nil {
					return fmt.Errorf("expected NodeSet %q was not found in Elasticsearch spec", expectedNodeSet.Name)
				}
				if err := ensureNodeSetResourcesMatchExpected(expectedNodeSet.Resources, actualNodeSet.Resources); err != nil {
					return fmt.Errorf("NodeSet %q resources mismatch: %w", expectedNodeSet.Name, err)
				}
			}

			return nil
		}),
	}
}

// checkAutoscaledPodsResourcesStep verifies that autoscaled pods use the expected CPU/memory
// requests and limits from their corresponding NodeSet resources.
func checkAutoscaledPodsResourcesStep(k8sClient *test.K8sClient, expectedBuilder *elasticsearch.Builder, nodeSetNames ...string) test.Step {
	return test.Step{
		Name: "Autoscaled pods should have expected CPU/memory resources",
		Test: test.Eventually(func() error {
			expectedES := expectedBuilder.GetExpectedElasticsearch()

			var actualES esv1.Elasticsearch
			if err := k8sClient.Client.Get(context.Background(), k8s.ExtractNamespacedName(&expectedES), &actualES); err != nil {
				return err
			}

			nodeSetsToCheck := make(map[string]struct{}, len(nodeSetNames))
			for _, nodeSetName := range nodeSetNames {
				nodeSetsToCheck[nodeSetName] = struct{}{}
			}

			for _, nodeSet := range actualES.Spec.NodeSets {
				if _, shouldCheck := nodeSetsToCheck[nodeSet.Name]; !shouldCheck || nodeSet.Count == 0 {
					continue
				}
				if err := ensureCPUAndMemorySet(nodeSet.Name, nodeSet.Resources); err != nil {
					return err
				}

				pods, err := k8sClient.GetPods(test.ESPodListOptionsByNodeSet(actualES.Namespace, actualES.Name, nodeSet.Name)...)
				if err != nil {
					return err
				}
				if len(pods) == 0 {
					return fmt.Errorf("expected pods for NodeSet %q but none were found", nodeSet.Name)
				}

				for _, pod := range pods {
					podResources, err := getElasticsearchContainerResources(pod)
					if err != nil {
						return err
					}

					if err := ensureResourceEqual(pod.Name, "cpu request", *nodeSet.Resources.Requests.CPU, podResources.Requests[corev1.ResourceCPU]); err != nil {
						return err
					}
					if err := ensureResourceEqual(pod.Name, "memory request", *nodeSet.Resources.Requests.Memory, podResources.Requests[corev1.ResourceMemory]); err != nil {
						return err
					}
					if err := ensureResourceEqual(pod.Name, "cpu limit", *nodeSet.Resources.Limits.CPU, podResources.Limits[corev1.ResourceCPU]); err != nil {
						return err
					}
					if err := ensureResourceEqual(pod.Name, "memory limit", *nodeSet.Resources.Limits.Memory, podResources.Limits[corev1.ResourceMemory]); err != nil {
						return err
					}
				}
			}

			return nil
		}),
	}
}

// ensureNodeSetResourcesMatchExpected checks expected resources as a subset of actual resources.
// Autoscaling may populate additional CPU/memory fields over time, so we only enforce fields
// explicitly expected by this test.
func ensureNodeSetResourcesMatchExpected(expected, actual commonv1.Resources) error {
	for _, check := range []struct {
		name     string
		expected *resource.Quantity
		actual   *resource.Quantity
	}{
		{name: "requests.cpu", expected: expected.Requests.CPU, actual: actual.Requests.CPU},
		{name: "requests.memory", expected: expected.Requests.Memory, actual: actual.Requests.Memory},
		{name: "limits.cpu", expected: expected.Limits.CPU, actual: actual.Limits.CPU},
		{name: "limits.memory", expected: expected.Limits.Memory, actual: actual.Limits.Memory},
	} {
		if check.expected == nil {
			continue
		}
		if check.actual == nil {
			return fmt.Errorf("%s is missing in actual NodeSet resources", check.name)
		}
		if !check.expected.Equal(*check.actual) {
			return fmt.Errorf("%s expected %s, got %s", check.name, check.expected.String(), check.actual.String())
		}
	}
	return nil
}

// ensureCPUAndMemorySet verifies CPU and memory requests/limits are explicitly set.
func ensureCPUAndMemorySet(nodeSetName string, resources commonv1.Resources) error {
	for _, check := range []struct {
		name  string
		value *resource.Quantity
	}{
		{name: "requests.cpu", value: resources.Requests.CPU},
		{name: "requests.memory", value: resources.Requests.Memory},
		{name: "limits.cpu", value: resources.Limits.CPU},
		{name: "limits.memory", value: resources.Limits.Memory},
	} {
		if check.value == nil || check.value.IsZero() {
			return fmt.Errorf("NodeSet %q is missing %s", nodeSetName, check.name)
		}
	}
	return nil
}

// getElasticsearchContainerResources returns resources of the Elasticsearch container from a pod.
func getElasticsearchContainerResources(pod corev1.Pod) (corev1.ResourceRequirements, error) {
	for _, container := range pod.Spec.Containers {
		if container.Name == esv1.ElasticsearchContainerName {
			return container.Resources, nil
		}
	}
	return corev1.ResourceRequirements{}, fmt.Errorf("pod %q does not contain an Elasticsearch container", pod.Name)
}

// ensureResourceEqual verifies that the actual pod resource value exists and matches expected.
func ensureResourceEqual(podName, resourceName string, expected resource.Quantity, actual resource.Quantity) error {
	if actual.IsZero() {
		return fmt.Errorf("pod %q %s is missing", podName, resourceName)
	}
	if !expected.Equal(actual) {
		return fmt.Errorf("pod %q %s expected %s, got %s", podName, resourceName, expected.String(), actual.String())
	}
	return nil
}
