// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch/autoscaling"
	"github.com/stretchr/testify/require"
)

// TestAutoscalingLegacy ensures that the operator is compatible with the autoscaling Elasticsearch API when an annotation
// is used to configure the autoscaling policies.
// The purpose of this test is only to assess that there is no regression at the API level. It only relies on the
// fixed decider to generate scaling events, other deciders, like storage deciders or ML deciders are not exercised.
// Note that the autoscaling annotation is now deprecated in favor of the ElasticsearchAutoscaler custom resource.
func TestAutoscalingLegacy(t *testing.T) {
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
			newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
			// Autoscaling controller should eventually update the data node count to its min. value.
			newNodeSet("data-ingest", []string{"data", "ingest"}, 2, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, newPVC("10Gi", storageClass)),
			// ML node count should still be 0.
			newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
		)
	autoscalingSpecBuilder := autoscaling.NewAutoscalingBuilder(t, k8s.ExtractNamespacedName(&esBuilder.Elasticsearch)).
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
			NodeCountRange: v1alpha1.CountRange{Min: 0, Max: 2},
		})
	esBuilder = esBuilder.WithAnnotation(esv1.ElasticsearchAutoscalingSpecAnnotationName, autoscalingSpecBuilder.ToJSON())

	// scaleUpStorage uses the fixed decider to trigger a scale up of the data tier up to its max memory limit and 3 nodes.
	expectedDataPVC := newPVC("20Gi", storageClass)
	scaleUpStorage := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		// Only request 19gb, the operator adds a capacity margin of 5% to account for reserved fs space, we don't want to exceed 3 nodes of 20Gi in this test.
		autoscalingSpecBuilder.WithFixedDecider("data-ingest", map[string]string{"storage": "19gb", "nodes": "3"}).ToJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	// scaleUpML uses the fixed decider to trigger the creation of a ML node.
	scaleUpML := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		autoscalingSpecBuilder.WithFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "1"}).ToJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 1, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, initialPVC),
	)

	// scaleDownML use the fixed decider to trigger the scale down, and thus the deletion, of the ML node previously created.
	scaleDownML := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		autoscalingSpecBuilder.WithFixedDecider("ml", map[string]string{"memory": "0gb", "nodes": "0"}).ToJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	esWithLicense := test.LicenseTestBuilder()
	esWithLicense.BuildingThis = esBuilder

	autoscalingCapacityTest := autoscaling.NewAutoscalingCapacityTest(esBuilder.Elasticsearch, k8sClient)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			WithStep(autoscalingCapacityTest).
			// Scale vertically and horizontally to add some storage capacity
			WithSteps(scaleUpStorage.UpgradeTestSteps(k)).
			WithSteps(scaleUpStorage.CheckK8sTestSteps(k)).
			WithSteps(scaleUpStorage.CheckStackTestSteps(k)).
			WithStep(autoscalingCapacityTest).
			// Scale vertically and horizontally to add some ml capacity
			WithSteps(scaleUpML.UpgradeTestSteps(k)).
			WithSteps(scaleUpML.CheckK8sTestSteps(k)).
			WithSteps(scaleUpML.CheckStackTestSteps(k)).
			WithStep(autoscalingCapacityTest).
			// Scale ML tier back to 0 node
			WithSteps(scaleDownML.UpgradeTestSteps(k)).
			WithSteps(scaleDownML.CheckK8sTestSteps(k)).
			WithSteps(scaleDownML.CheckStackTestSteps(k)).
			WithStep(autoscalingCapacityTest)
	}

	test.Sequence(nil, stepsFn, esWithLicense).RunSequential(t)
}
