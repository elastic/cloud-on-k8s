// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
)

func TestReconcileElasticsearch(t *testing.T) {
	tests := []struct {
		name                             string
		initialES                        esv1.Elasticsearch
		nextClusterResources             v1alpha1.ClusterResources
		wantNodeSetCount                 int32
		wantCPURequest                   string
		wantMemoryRequest                string
		wantCPULimit                     string
		wantMemoryLimit                  string
		wantStorageRequest               string
		wantPodTemplateRequestsNil       bool
		wantPodTemplateLimitsNil         bool
		wantPodTemplateExtraRequestKey   corev1.ResourceName
		wantPodTemplateExtraRequestValue string
	}{
		{
			name: "strips cpu/memory from pod template after writing nodeSet shorthand resources",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "hot",
							Count: 1,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("500m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("1000m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("750m"),
													corev1.ResourceMemory: resource.MustParse("1500Mi"),
												},
											},
										},
									},
								},
							},
							VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
								{
									ObjectMeta: metav1.ObjectMeta{Name: volume.ElasticsearchDataVolumeName},
									Spec: corev1.PersistentVolumeClaimSpec{
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-hot",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "hot", NodeCount: 3},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:     resource.MustParse("2000m"),
							corev1.ResourceMemory:  resource.MustParse("4Gi"),
							corev1.ResourceStorage: resource.MustParse("8Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("3000m"),
							corev1.ResourceMemory: resource.MustParse("6Gi"),
						},
					},
				},
			},
			wantNodeSetCount:           3,
			wantCPURequest:             "2000m",
			wantMemoryRequest:          "4Gi",
			wantCPULimit:               "3000m",
			wantMemoryLimit:            "6Gi",
			wantStorageRequest:         "8Gi",
			wantPodTemplateRequestsNil: true,
			wantPodTemplateLimitsNil:   true,
		},
		{
			name: "missing cpu recommendations preserves existing cpu values",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "warm",
							Count: 2,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("800m")),
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									CPU:    quantityPtr(resource.MustParse("1200m")),
									Memory: quantityPtr(resource.MustParse("3Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("900m"),
													corev1.ResourceMemory: resource.MustParse("1Gi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-warm",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "warm", NodeCount: 4},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("5Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("7Gi"),
						},
					},
				},
			},
			wantNodeSetCount:           4,
			wantCPURequest:             "800m",
			wantMemoryRequest:          "5Gi",
			wantCPULimit:               "1200m",
			wantMemoryLimit:            "7Gi",
			wantPodTemplateRequestsNil: true,
			wantPodTemplateLimitsNil:   true,
		},
		{
			name: "missing cpu recommendations preserves nil cpu values",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "cold",
							Count: 1,
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									Memory: quantityPtr(resource.MustParse("1Gi")),
								},
								Limits: commonv1.ResourceAllocations{
									Memory: quantityPtr(resource.MustParse("2Gi")),
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:    resource.MustParse("700m"),
													corev1.ResourceMemory: resource.MustParse("1400Mi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-cold",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "cold", NodeCount: 2},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("3Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			wantNodeSetCount:           2,
			wantCPURequest:             "",
			wantMemoryRequest:          "3Gi",
			wantCPULimit:               "",
			wantMemoryLimit:            "4Gi",
			wantPodTemplateRequestsNil: true,
			wantPodTemplateLimitsNil:   true,
		},
		{
			name: "preserves non cpu/memory keys on the main container",
			initialES: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:  "hot",
							Count: 1,
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU:              resource.MustParse("500m"),
													corev1.ResourceMemory:           resource.MustParse("1Gi"),
													corev1.ResourceEphemeralStorage: resource.MustParse("2Gi"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			nextClusterResources: v1alpha1.ClusterResources{
				{
					Name: "policy-hot",
					NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
						{Name: "hot", NodeCount: 2},
					},
					NodeResources: v1alpha1.NodeResources{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1500m"),
							corev1.ResourceMemory: resource.MustParse("3Gi"),
						},
					},
				},
			},
			wantNodeSetCount:                 2,
			wantCPURequest:                   "1500m",
			wantMemoryRequest:                "3Gi",
			wantPodTemplateLimitsNil:         true,
			wantPodTemplateExtraRequestKey:   corev1.ResourceEphemeralStorage,
			wantPodTemplateExtraRequestValue: "2Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.initialES.DeepCopy()

			err := reconcileElasticsearch(logr.Discard(), es, tt.nextClusterResources)
			require.NoError(t, err)
			require.Len(t, es.Spec.NodeSets, 1)

			nodeSet := es.Spec.NodeSets[0]
			assert.Equal(t, tt.wantNodeSetCount, nodeSet.Count)
			assertQuantityPointerEqual(t, tt.wantCPURequest, nodeSet.Resources.Requests.CPU)
			assertQuantityPointerEqual(t, tt.wantMemoryRequest, nodeSet.Resources.Requests.Memory)
			assertQuantityPointerEqual(t, tt.wantCPULimit, nodeSet.Resources.Limits.CPU)
			assertQuantityPointerEqual(t, tt.wantMemoryLimit, nodeSet.Resources.Limits.Memory)

			if tt.wantStorageRequest != "" {
				require.Len(t, nodeSet.VolumeClaimTemplates, 1)
				assert.True(
					t,
					resource.MustParse(tt.wantStorageRequest).Equal(nodeSet.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]),
				)
			}

			mainContainer := getMainContainer(nodeSet)
			require.NotNil(t, mainContainer)
			_, hasCPURequest := mainContainer.Resources.Requests[corev1.ResourceCPU]
			_, hasMemRequest := mainContainer.Resources.Requests[corev1.ResourceMemory]
			_, hasCPULimit := mainContainer.Resources.Limits[corev1.ResourceCPU]
			_, hasMemLimit := mainContainer.Resources.Limits[corev1.ResourceMemory]
			assert.False(t, hasCPURequest, "pod template main container CPU request must be stripped")
			assert.False(t, hasMemRequest, "pod template main container memory request must be stripped")
			assert.False(t, hasCPULimit, "pod template main container CPU limit must be stripped")
			assert.False(t, hasMemLimit, "pod template main container memory limit must be stripped")
			if tt.wantPodTemplateRequestsNil {
				assert.Nil(t, mainContainer.Resources.Requests, "expected requests map to be nil after stripping all keys")
			}
			if tt.wantPodTemplateLimitsNil {
				assert.Nil(t, mainContainer.Resources.Limits, "expected limits map to be nil after stripping all keys")
			}
			if tt.wantPodTemplateExtraRequestKey != "" {
				got, ok := mainContainer.Resources.Requests[tt.wantPodTemplateExtraRequestKey]
				require.True(t, ok, "expected non-CPU/memory request key %q to be preserved", tt.wantPodTemplateExtraRequestKey)
				assert.True(t, resource.MustParse(tt.wantPodTemplateExtraRequestValue).Equal(got))
			}
		})
	}
}

// TestReconcileElasticsearch_MigratesFromPodTemplateAndIsIdempotent simulates an operator upgrade from a
// version that only wrote autoscaler-managed CPU/memory on the PodTemplate container to the current version
// that writes the shorthand NodeSet.Resources field. After the first reconcile the shorthand must reflect
// the autoscaler recommendation and the previously-written PodTemplate CPU/memory entries must be removed
// so the validating webhook does not fire.
func TestReconcileElasticsearch_MigratesFromPodTemplateAndIsIdempotent(t *testing.T) {
	next := v1alpha1.ClusterResources{
		{
			Name: "policy-hot",
			NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{
				{Name: "hot", NodeCount: 2},
			},
			NodeResources: v1alpha1.NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1500m"),
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"),
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
			},
		},
	}

	// Initial state mimics an older operator that wrote autoscaler outputs to the PodTemplate container and
	// left NodeSet.Resources unset.
	es := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:  "hot",
					Count: 1,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("500m"),
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	require.NoError(t, reconcileElasticsearch(logr.Discard(), &es, next))

	nodeSet := es.Spec.NodeSets[0]
	assert.Equal(t, int32(2), nodeSet.Count)
	assertQuantityPointerEqual(t, "1500m", nodeSet.Resources.Requests.CPU)
	assertQuantityPointerEqual(t, "3Gi", nodeSet.Resources.Requests.Memory)
	assertQuantityPointerEqual(t, "2000m", nodeSet.Resources.Limits.CPU)
	assertQuantityPointerEqual(t, "3Gi", nodeSet.Resources.Limits.Memory)

	// PodTemplate CPU/memory entries left by the previous operator must be removed so the shorthand stays
	// the single source of truth and the validating webhook stops emitting an admission warning every
	// reconcile. Effective container resources are unchanged because WithResourcesAndOverrides applies the
	// new shorthand at pod-template-build time, so this does not trigger a rolling restart on its own.
	mainContainer := getMainContainer(nodeSet)
	require.NotNil(t, mainContainer)
	assert.Nil(t, mainContainer.Resources.Requests, "expected requests to be nil after stripping cpu/memory")
	assert.Nil(t, mainContainer.Resources.Limits, "expected limits to be nil after stripping cpu/memory")

	// A second reconcile with the same recommendation must be an in-memory no-op to keep upstream Update
	// calls from persistently dirtying the Elasticsearch custom resource.
	before := es.DeepCopy()
	require.NoError(t, reconcileElasticsearch(logr.Discard(), &es, next))
	assert.Equal(t, before.Spec, es.Spec)
}

// TestReconcileElasticsearch_NonAutoscaledNodeSetDoesNotPersistEmptyResourcesStub covers the
// upgrade-path scenario where a cluster with a dedicated `master` NodeSet that
// uses the legacy podTemplate.spec.containers[].resources path alongside autoscaled NodeSets
// (`di`, `ml`) that the autoscaler manages. After reconciliation the master
// NodeSet must:
//
//   - keep its zero-valued in-memory Resources (the autoscaler does not produce a recommendation
//     for it and reconcileElasticsearch must skip it without writing the shorthand), AND
//   - serialize without an empty `resources: { limits: {}, requests: {} }` stub when the
//     Elasticsearch custom resource is round-tripped through JSON.
func TestReconcileElasticsearch_NonAutoscaledNodeSetDoesNotPersistEmptyResourcesStub(t *testing.T) {
	masterContainerCPU := resource.MustParse("1")
	masterContainerMem := resource.MustParse("2Gi")

	// Initial state: master uses the legacy podTemplate path with no shorthand; di and ml are
	// autoscaled and currently sized to the previous recommendation.
	es := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:  "master",
					Count: 3,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    masterContainerCPU,
											corev1.ResourceMemory: masterContainerMem,
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    masterContainerCPU,
											corev1.ResourceMemory: masterContainerMem,
										},
									},
								},
							},
						},
					},
				},
				{
					Name:  "di",
					Count: 2,
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    quantityPtr(resource.MustParse("1")),
							Memory: quantityPtr(resource.MustParse("4Gi")),
						},
					},
				},
				{
					Name:  "ml",
					Count: 1,
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{
							CPU:    quantityPtr(resource.MustParse("500m")),
							Memory: quantityPtr(resource.MustParse("2Gi")),
						},
					},
				},
			},
		},
	}

	// The autoscaler only produces recommendations for di and ml; master has no entry.
	next := v1alpha1.ClusterResources{
		{
			Name:             "policy-di",
			NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{{Name: "di", NodeCount: 3}},
			NodeResources: v1alpha1.NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			Name:             "policy-ml",
			NodeSetNodeCount: v1alpha1.NodeSetNodeCountList{{Name: "ml", NodeCount: 2}},
			NodeResources: v1alpha1.NodeResources{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	}

	require.NoError(t, reconcileElasticsearch(logr.Discard(), &es, next))

	require.Len(t, es.Spec.NodeSets, 3)
	master := es.Spec.NodeSets[0]
	require.Equal(t, "master", master.Name)

	// In-memory: the autoscaler must not have touched master's Resources or its podTemplate
	// container resources.
	assert.True(
		t,
		master.Resources.IsEmpty(),
		"master NodeSet shorthand Resources must remain empty when the autoscaler does not touch it; got %+v",
		master.Resources,
	)
	mainContainer := getMainContainer(master)
	require.NotNil(t, mainContainer)
	assert.True(
		t,
		masterContainerCPU.Equal(mainContainer.Resources.Requests[corev1.ResourceCPU]),
		"master podTemplate CPU request must be preserved",
	)
	assert.True(
		t,
		masterContainerMem.Equal(mainContainer.Resources.Requests[corev1.ResourceMemory]),
		"master podTemplate memory request must be preserved",
	)

	// Serialized: the persisted CR must not carry the empty `resources: {}` stub on master,
	// otherwise users see two different ways of expressing container resources side by side
	// even though they only ever set the legacy path.
	encoded, err := json.Marshal(es)
	require.NoError(t, err)

	var roundTrip map[string]any
	require.NoError(t, json.Unmarshal(encoded, &roundTrip))
	spec, ok := roundTrip["spec"].(map[string]any)
	require.True(t, ok, "expected spec object in serialized Elasticsearch")
	nodeSets, ok := spec["nodeSets"].([]any)
	require.True(t, ok, "expected nodeSets array in serialized spec")
	require.Len(t, nodeSets, 3)

	masterEncoded, ok := nodeSets[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "master", masterEncoded["name"])
	_, hasResources := masterEncoded["resources"]
	assert.False(
		t,
		hasResources,
		"non-autoscaled master NodeSet must not serialize an empty resources stub; encoded: %s",
		string(encoded),
	)

	// the autoscaled NodeSets do still serialize their (non-empty) shorthand.
	for _, idx := range []int{1, 2} {
		ns, ok := nodeSets[idx].(map[string]any)
		require.True(t, ok)
		_, hasResources := ns["resources"]
		assert.True(
			t,
			hasResources,
			"autoscaled NodeSet %q must serialize its shorthand resources",
			ns["name"],
		)
	}
}

// getMainContainer returns the Elasticsearch main container from a NodeSet pod template.
func getMainContainer(nodeSet esv1.NodeSet) *corev1.Container {
	for i := range nodeSet.PodTemplate.Spec.Containers {
		container := nodeSet.PodTemplate.Spec.Containers[i]
		if container.Name == esv1.ElasticsearchContainerName {
			return &container
		}
	}
	return nil
}

// assertQuantityPointerEqual compares a quantity pointer value with an expected quantity string.
// An empty expected string asserts that the pointer is nil.
func assertQuantityPointerEqual(t *testing.T, expected string, current *resource.Quantity) {
	t.Helper()
	if expected == "" {
		assert.Nil(t, current)
		return
	}
	require.NotNil(t, current)
	assert.True(t, resource.MustParse(expected).Equal(*current))
}

// quantityPtr returns a pointer to a copy of the provided quantity value.
func quantityPtr(quantity resource.Quantity) *resource.Quantity {
	q := quantity
	return &q
}
