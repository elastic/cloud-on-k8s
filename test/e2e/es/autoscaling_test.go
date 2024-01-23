// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch/autoscaling"
)

// TestAutoscaling ensures that the operator is compatible with the autoscaling Elasticsearch API.
// The purpose of this test is only to assess that there is no regression at the API level. It only relies on the
// fixed decider to generate scaling events, other deciders, like storage deciders or ML deciders are not exercised.
// Note that only the node count and the memory limit of the deployed Pods are validated for now, see https://github.com/elastic/cloud-on-k8s/issues/4411.
func TestAutoscaling(t *testing.T) {
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
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	// scaleUpML uses the fixed decider to trigger the creation of a ML node.
	esaScaleUpML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "1"})
	esScaleUpML := esBuilder.DeepCopy().WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 1, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, initialPVC),
	)

	// esaScaleUpLimitedML uses the fixed decider to request the creation of more nodes than allowed.
	esaScaleUpLimitedML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "2"})

	// scaleDownML use the fixed decider to trigger the scale down, and thus the deletion, of the ML node previously created.
	esaScaleDownML := autoscalingBuilder.DeepCopy().WithFixedDecider("ml", map[string]string{"memory": "0gb", "nodes": "0"})
	esScaleDownML := esBuilder.DeepCopy().WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
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

			// Scale vertically and horizontally to add some storage capacity
			WithSteps(esaScaleUpStorageBuilder.UpgradeTestSteps(k)).
			WithSteps(esScaleUpStorageBuilder.CheckK8sTestSteps(k)).
			WithSteps(esScaleUpStorageBuilder.CheckStackTestSteps(k)).
			WithStep(autoscalingCapacityTest).

			// Scale vertically and horizontally to add some ml capacity
			WithSteps(esaScaleUpML.UpgradeTestSteps(k)).
			WithSteps(esScaleUpML.CheckK8sTestSteps(k)).
			WithSteps(esScaleUpML.CheckStackTestSteps(k)).
			WithStep(autoscalingCapacityTest).

			// Scale horizontally up to the limit capacity
			WithSteps(esaScaleUpLimitedML.UpgradeTestSteps(k)).
			// Autoscaler should be limited
			WithStep(autoscalingBuilder.NewAutoscalingStatusTestBuilder(k8sClient).
				ShouldBeActive().ShouldBeHealthy().ShouldBeOnline().ShouldBeLimited().ToStep(),
			).

			// Scale ML tier back to 0 node
			WithSteps(esaScaleDownML.UpgradeTestSteps(k)).
			WithSteps(esScaleDownML.CheckK8sTestSteps(k)).
			WithSteps(esScaleDownML.CheckStackTestSteps(k)).
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
			Data: map[string]interface{}{esv1.NodeRoles: roles},
		},
		Count:                count,
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
