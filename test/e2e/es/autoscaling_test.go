// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build es e2e

package es

import (
	"encoding/json"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// This autoscaling test requires a storage class which supports volume expansion.
	storageClass, err := getResizeableStorageClass(test.NewK8sClientOrFatal().Client)
	require.NoError(t, err)
	if storageClass == "" {
		t.Skip("No storage class allowing volume expansion found. Skipping the test.")
	}

	// The test sequence involves 2 tiers:
	// * A data tier with 2 initial nodes.
	// * A ML tier with no node initially started.
	autoscalingSpecBuilder := NewAutoscalingSpecBuilder(t).
		withPolicy("data-ingest", []string{"data", "ingest"}, esv1.AutoscalingResources{
			CPURange:       &esv1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
			MemoryRange:    &esv1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("4Gi")},
			StorageRange:   &esv1.QuantityRange{Min: resource.MustParse("10Gi"), Max: resource.MustParse("20Gi")},
			NodeCountRange: esv1.CountRange{Min: 2, Max: 4},
		}).
		withPolicy("ml", []string{"ml"}, esv1.AutoscalingResources{
			CPURange:       &esv1.QuantityRange{Min: resource.MustParse("1"), Max: resource.MustParse("2")},
			MemoryRange:    &esv1.QuantityRange{Min: resource.MustParse("2Gi"), Max: resource.MustParse("4Gi")},
			StorageRange:   &esv1.QuantityRange{Min: resource.MustParse("1Gi"), Max: resource.MustParse("1Gi")},
			NodeCountRange: esv1.CountRange{Min: 0, Max: 2},
		})

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
		WithAnnotation(esv1.ElasticsearchAutoscalingSpecAnnotationName, autoscalingSpecBuilder.toJSON()).
		WithExpectedNodeSets(
			newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
			// Autoscaling controller should eventually update the data node count to its min. value.
			newNodeSet("data-ingest", []string{"data", "ingest"}, 2, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, newPVC("10Gi", storageClass)),
			// ML node count should still be 0.
			newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
		)

	// scaleUpStorage uses the fixed decider to trigger a scale up of the data tier up to its max memory limit and 3 nodes.
	expectedDataPVC := newPVC("20Gi", storageClass)
	scaleUpStorage := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		autoscalingSpecBuilder.withFixedDecider("data-ingest", map[string]string{"storage": "20gb", "nodes": "3"}).toJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	// scaleUpML uses the fixed decider to trigger the creation of a ML node.
	scaleUpML := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		autoscalingSpecBuilder.withFixedDecider("ml", map[string]string{"memory": "4gb", "nodes": "1"}).toJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 1, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, initialPVC),
	)

	// scaleDownML use the fixed decider to trigger the scale down, and thus the deletion, of the ML node previously created.
	scaleDownML := esBuilder.DeepCopy().WithAnnotation(
		esv1.ElasticsearchAutoscalingSpecAnnotationName,
		autoscalingSpecBuilder.withFixedDecider("ml", map[string]string{"memory": "0gb", "nodes": "0"}).toJSON(),
	).WithExpectedNodeSets(
		newNodeSet("master", []string{"master"}, 1, corev1.ResourceList{corev1.ResourceMemory: nodespec.DefaultMemoryLimits}, initialPVC),
		newNodeSet("data-ingest", []string{"data", "ingest"}, 3, corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, expectedDataPVC),
		newNodeSet("ml", []string{"ml"}, 0, corev1.ResourceList{}, initialPVC),
	)

	esWithLicense := test.LicenseTestBuilder()
	esWithLicense.BuildingThis = esBuilder

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{}.
			// Scale vertically and horizontally to add some storage capacity
			WithSteps(scaleUpStorage.UpgradeTestSteps(k)).
			WithSteps(scaleUpStorage.CheckK8sTestSteps(k)).
			WithSteps(scaleUpStorage.CheckStackTestSteps(k)).
			// Scale vertically and horizontally to add some ml capacity
			WithSteps(scaleUpML.UpgradeTestSteps(k)).
			WithSteps(scaleUpML.CheckK8sTestSteps(k)).
			WithSteps(scaleUpML.CheckStackTestSteps(k)).
			// Scale ML tier back to 0 node
			WithSteps(scaleDownML.UpgradeTestSteps(k)).
			WithSteps(scaleDownML.CheckK8sTestSteps(k)).
			WithSteps(scaleDownML.CheckStackTestSteps(k))
	}

	test.Sequence(nil, stepsFn, esWithLicense).RunSequential(t)
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
			Resources: corev1.ResourceRequirements{
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

// AutoscalingSpecBuilder helps to build and update autoscaling policies.
type AutoscalingSpecBuilder struct {
	t        *testing.T
	policies map[string]esv1.AutoscalingPolicySpec
}

func NewAutoscalingSpecBuilder(t *testing.T) *AutoscalingSpecBuilder {
	return &AutoscalingSpecBuilder{
		t:        t,
		policies: make(map[string]esv1.AutoscalingPolicySpec),
	}
}

// withPolicy adds or replaces an autoscaling policy.
func (ab *AutoscalingSpecBuilder) withPolicy(policy string, roles []string, resources esv1.AutoscalingResources) *AutoscalingSpecBuilder {
	ab.policies[policy] = esv1.AutoscalingPolicySpec{
		NamedAutoscalingPolicy: esv1.NamedAutoscalingPolicy{
			Name: policy,
			AutoscalingPolicy: esv1.AutoscalingPolicy{
				Roles:    roles,
				Deciders: make(map[string]esv1.DeciderSettings),
			},
		},
		AutoscalingResources: resources,
	}
	if stringsutil.StringInSlice("ml", roles) {
		// Disable ML scale down delay
		ab.policies[policy].Deciders["ml"] = map[string]string{"down_scale_delay": "0"}
	}
	return ab
}

// withFixedDecider set a setting for the fixed decider on an already existing policy.
func (ab *AutoscalingSpecBuilder) withFixedDecider(policy string, fixedDeciderSettings map[string]string) *AutoscalingSpecBuilder {
	policySpec, exists := ab.policies[policy]
	if !exists {
		ab.t.Fatalf("fixed decider must be set on an existing policy")
	}
	policySpec.Deciders["fixed"] = fixedDeciderSettings
	ab.policies[policy] = policySpec
	return ab
}

// toJSON converts the autoscaling policy into JSON.
func (ab *AutoscalingSpecBuilder) toJSON() string {
	spec := esv1.AutoscalingSpec{}
	for _, policySpec := range ab.policies {
		spec.AutoscalingPolicySpecs = append(spec.AutoscalingPolicySpecs, policySpec)
	}
	bytes, err := json.Marshal(spec)
	if err != nil {
		ab.t.Fatalf("can't serialize autoscaling spec, err: %s", err)
	}
	return string(bytes)
}
