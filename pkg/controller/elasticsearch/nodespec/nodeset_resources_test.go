// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackconfig"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func elasticsearchContainerResources(pod corev1.PodTemplateSpec) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == esv1.ElasticsearchContainerName {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func requireQuantityEqual(t *testing.T, list corev1.ResourceList, name corev1.ResourceName, want string) {
	t.Helper()
	got, ok := list[name]
	require.True(t, ok, "missing resource %s", name)
	wantQ := resource.MustParse(want)
	require.True(t, got.Equal(wantQ), "resource %s: got %s want %s", name, got.String(), wantQ.String())
}

func testElasticsearchForNodeSet(nodeSet esv1.NodeSet) esv1.Elasticsearch {
	es := newEsSampleBuilder().withVersion("8.14.0").build()
	es.Spec.NodeSets = []esv1.NodeSet{nodeSet}
	return es
}

func TestNodeSetResources_BuildPodTemplateSpec(t *testing.T) {
	scriptsCM := func(es esv1.Elasticsearch) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      esv1.ScriptsConfigMap(es.Name),
			},
		}
	}

	basePodTemplate := func(esContainer corev1.Container) corev1.PodTemplateSpec {
		return corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "additional-container"},
					esContainer,
				},
			},
		}
	}

	esContainerMinimal := corev1.Container{Name: esv1.ElasticsearchContainerName}
	esContainerWithPodResources := corev1.Container{
		Name: esv1.ElasticsearchContainerName,
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
	}

	for _, tt := range []struct {
		name            string
		nodeSet         esv1.NodeSet
		assertResources func(t *testing.T, got corev1.ResourceRequirements)
	}{
		{
			name: "happy_path_nodeset_resources_pod_template_unset",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("1500m")),
						Memory: new(resource.MustParse("4Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("4Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceCPU, "1500m")
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "4Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "2")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "4Gi")
			},
		},
		{
			name: "defaults_when_nodeset_and_pod_resources_unset",
			nodeSet: esv1.NodeSet{
				Name:                 "nodeset-1",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate:          basePodTemplate(esContainerMinimal),
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, DefaultResources, got)
			},
		},
		{
			name: "nodeset_overrides_pod_template_resources",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerWithPodResources),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("8Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("8Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceCPU, "2")
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "8Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "2")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "8Gi")
			},
		},
		{
			name: "pod_template_only_no_nodeset_overrides",
			nodeSet: esv1.NodeSet{
				Name:                 "nodeset-1",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate:          basePodTemplate(esContainerWithPodResources),
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				require.Equal(t, esContainerWithPodResources.Resources, got)
			},
		},
		{
			name: "nodeset_partial_override_preserves_other_keys_from_pod_template",
			nodeSet: esv1.NodeSet{
				Name:   "nodeset-1",
				Count:  1,
				Config: &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(corev1.Container{
					Name: esv1.ElasticsearchContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("100m"),
							corev1.ResourceMemory:           resource.MustParse("2Gi"),
							corev1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:              resource.MustParse("500m"),
							corev1.ResourceMemory:           resource.MustParse("2Gi"),
							corev1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
						},
					},
				}),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Limits: commonv1.ResourceAllocations{
						CPU: new(resource.MustParse("1500m")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceCPU, "100m")
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "2Gi")
				requireQuantityEqual(t, got.Requests, corev1.ResourceEphemeralStorage, "10Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "1500m")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "2Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceEphemeralStorage, "10Gi")
			},
		},
		{
			name: "nodeset_partial_override_skips_defaults",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU: new(resource.MustParse("250m")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU: new(resource.MustParse("1")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceCPU, "250m")
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "1")
				_, hasMemReq := got.Requests[corev1.ResourceMemory]
				_, hasMemLim := got.Limits[corev1.ResourceMemory]
				require.False(t, hasMemReq, "memory request should not be set when shorthand skips operator defaults")
				require.False(t, hasMemLim, "memory limit should not be set when shorthand skips operator defaults")
			},
		},
		{
			name: "nodeset_memory_request_only_leaves_limit_nil_for_api_server_defaulting",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						Memory: new(resource.MustParse("4Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "4Gi")
				require.Nil(t, got.Limits, "limits should stay nil so the API server's limit↔request defaulting can apply")
				_, hasCPUReq := got.Requests[corev1.ResourceCPU]
				require.False(t, hasCPUReq)
			},
		},
		{
			name: "nodeset_limits_only_leaves_requests_nil_for_guaranteed_qos",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Limits: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("1")),
						Memory: new(resource.MustParse("3Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "1")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "3Gi")
				require.Nil(t, got.Requests, "requests should stay nil so the API server defaults them to limits (Guaranteed QoS)")
			},
		},
		{
			name: "nodeset_override_only_memory_cpu_from_defaults",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Limits: commonv1.ResourceAllocations{
						Memory: new(resource.MustParse("6Gi")),
					},
					Requests: commonv1.ResourceAllocations{
						Memory: new(resource.MustParse("6Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "6Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "6Gi")
				_, hasCPUReq := got.Requests[corev1.ResourceCPU]
				_, hasCPULim := got.Limits[corev1.ResourceCPU]
				require.False(t, hasCPUReq, "CPU request should not be set when not in defaults or overrides")
				require.False(t, hasCPULim, "CPU limit should not be set when not in defaults or overrides")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			es := testElasticsearchForNodeSet(tt.nodeSet)
			client := k8s.NewFakeClient(scriptsCM(es))

			nodeSet := es.Spec.NodeSets[0]
			ver, err := version.Parse(es.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(
				es.Name, ver, corev1.IPv4Protocol, es.Spec.HTTP,
				*nodeSet.Config, nil, false, false, nodeSet.ZoneAwareness != nil, false,
			)
			require.NoError(t, err)

			template, err := BuildPodTemplateSpec(
				t.Context(), client, es, nodeSet, cfg,
				nil, false, stackconfig.PolicyConfig{}, metadata.Metadata{}, "", false,
			)
			require.NoError(t, err)

			res, ok := elasticsearchContainerResources(template)
			require.True(t, ok, "elasticsearch container not found")
			tt.assertResources(t, res)

			additional, ok := resourceForContainerName(template, "additional-container")
			require.True(t, ok)
			require.Empty(t, additional.Requests)
			require.Empty(t, additional.Limits)
		})
	}
}

func resourceForContainerName(pod corev1.PodTemplateSpec, name string) (corev1.ResourceRequirements, bool) {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return pod.Spec.Containers[i].Resources, true
		}
	}
	return corev1.ResourceRequirements{}, false
}

func TestNodeSetResources_DefaultResourcesGlobalUnmodified(t *testing.T) {
	snapshot := *DefaultResources.DeepCopy()

	nodeSet := esv1.NodeSet{
		Name:  "nodeset-1",
		Count: 1,
		Config: &commonv1.Config{
			Data: map[string]any{"node.roles": []string{"master", "data"}},
		},
		PodTemplate: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}},
			},
		},
		Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
			Requests: commonv1.ResourceAllocations{
				CPU: new(resource.MustParse("1")),
			},
			Limits: commonv1.ResourceAllocations{
				CPU: new(resource.MustParse("2")),
			},
		}},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
	}
	es := testElasticsearchForNodeSet(nodeSet)
	client := k8s.NewFakeClient(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ScriptsConfigMap(es.Name)},
	})

	nodeSet = es.Spec.NodeSets[0]
	ver, err := version.Parse(es.Spec.Version)
	require.NoError(t, err)
	cfg, err := settings.NewMergedESConfig(
		es.Name, ver, corev1.IPv4Protocol, es.Spec.HTTP,
		*nodeSet.Config, nil, false, false, false, false,
	)
	require.NoError(t, err)

	_, err = BuildPodTemplateSpec(
		t.Context(), client, es, nodeSet, cfg,
		nil, false, stackconfig.PolicyConfig{}, metadata.Metadata{}, "", false,
	)
	require.NoError(t, err)

	require.Equal(t, snapshot, DefaultResources)
}

// TestBuildStatefulSet covers container resource propagation, nil-existing-StatefulSets safety,
// and the AppendDefaultPVCs -> ApplyStorageOverride ordering (storage shorthand). The ordering
// test is intentional: if the two calls in BuildStatefulSet were swapped, the "no user VCT" case
// would fail because ApplyStorageOverride would find no claim and become a no-op, then
// AppendDefaultPVCs would inject the default claim without the size.
func TestBuildStatefulSet(t *testing.T) {
	q10Gi := resource.MustParse("10Gi")

	makeVCT := func(name, size string) corev1.PersistentVolumeClaim {
		pvc := corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if size != "" {
			pvc.Spec.Resources.Requests = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(size)}
		}
		return pvc
	}

	findVCT := func(claims []corev1.PersistentVolumeClaim, name string) *corev1.PersistentVolumeClaim {
		for i := range claims {
			if claims[i].Name == name {
				return &claims[i]
			}
		}
		return nil
	}

	tests := []struct {
		name            string
		nodeSet         esv1.NodeSet
		existingSSets   es_sset.StatefulSetList
		assertResources func(t *testing.T, res corev1.ResourceRequirements)
		wantVCT         string
		wantStorage     string
	}{
		{
			name: "container resources: CPU and memory shorthand propagated",
			nodeSet: esv1.NodeSet{
				Name:  "nodeset-1",
				Count: 3,
				Config: &commonv1.Config{
					Data: map[string]any{"node.roles": []string{"master", "data"}},
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}},
					},
				},
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("1")),
						Memory: new(resource.MustParse("4Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("4Gi")),
					},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, res corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, res.Requests, corev1.ResourceCPU, "1")
				requireQuantityEqual(t, res.Requests, corev1.ResourceMemory, "4Gi")
				requireQuantityEqual(t, res.Limits, corev1.ResourceCPU, "2")
				requireQuantityEqual(t, res.Limits, corev1.ResourceMemory, "4Gi")
			},
		},
		{
			// A nil StatefulSetList is a valid input: GetByName on a nil slice must not panic.
			name: "nil existing StatefulSets: memory shorthand propagated without panic",
			nodeSet: esv1.NodeSet{
				Name:  "nodeset-1",
				Count: 1,
				Config: &commonv1.Config{
					Data: map[string]any{"node.roles": []string{"master", "data"}},
				},
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}},
					},
				},
				Resources: esv1.NodeSetResources{Resources: commonv1.Resources{
					Limits:   commonv1.ResourceAllocations{Memory: new(resource.MustParse("3Gi"))},
					Requests: commonv1.ResourceAllocations{Memory: new(resource.MustParse("3Gi"))},
				}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			existingSSets: nil,
			assertResources: func(t *testing.T, res corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, res.Requests, corev1.ResourceMemory, "3Gi")
				requireQuantityEqual(t, res.Limits, corev1.ResourceMemory, "3Gi")
			},
		},
		{
			// AppendDefaultPVCs injects elasticsearch-data; ApplyStorageOverride must run after.
			name: "storage shorthand: no user VCT, applied to injected default claim",
			nodeSet: esv1.NodeSet{
				Name:        "data",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"data"}}},
				PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}}}},
				Resources:   esv1.NodeSetResources{Storage: &q10Gi},
			},
			wantVCT:     "elasticsearch-data",
			wantStorage: "10Gi",
		},
		{
			// User declares the claim with a storage class but no size. AppendDefaultPVCs skips
			// injection (claim already present). ApplyStorageOverride applies the shorthand.
			name: "storage shorthand: user VCT with no size",
			nodeSet: esv1.NodeSet{
				Name:                 "data",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"data"}}},
				PodTemplate:          corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}}}},
				Resources:            esv1.NodeSetResources{Storage: &q10Gi},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{makeVCT("elasticsearch-data", "")},
			},
			wantVCT:     "elasticsearch-data",
			wantStorage: "10Gi",
		},
		{
			name: "storage shorthand: shorthand wins over VCT's own size",
			nodeSet: esv1.NodeSet{
				Name:                 "data",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"data"}}},
				PodTemplate:          corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}}}},
				Resources:            esv1.NodeSetResources{Storage: &q10Gi},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{makeVCT("elasticsearch-data", "5Gi")},
			},
			wantVCT:     "elasticsearch-data",
			wantStorage: "10Gi",
		},
		{
			// Single custom-named VCT: fallback path in StorageOverrideClaim.
			name: "storage shorthand: single custom-named VCT via fallback",
			nodeSet: esv1.NodeSet{
				Name:                 "data",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"data"}}},
				PodTemplate:          corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}}}},
				Resources:            esv1.NodeSetResources{Storage: &q10Gi},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{makeVCT("custom-data", "5Gi")},
			},
			wantVCT:     "custom-data",
			wantStorage: "10Gi",
		},
		{
			name: "storage shorthand: nil shorthand leaves VCT size unchanged",
			nodeSet: esv1.NodeSet{
				Name:                 "data",
				Count:                1,
				Config:               &commonv1.Config{Data: map[string]any{"node.roles": []string{"data"}}},
				PodTemplate:          corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}}}},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{makeVCT("elasticsearch-data", "5Gi")},
			},
			wantVCT:     "elasticsearch-data",
			wantStorage: "5Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := testElasticsearchForNodeSet(tt.nodeSet)
			client := k8s.NewFakeClient(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: es.Namespace, Name: esv1.ScriptsConfigMap(es.Name)},
			})

			ns := es.Spec.NodeSets[0]
			ver, err := version.Parse(es.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(
				es.Name, ver, corev1.IPv4Protocol, es.Spec.HTTP,
				*ns.Config, nil, false, false, false, false,
			)
			require.NoError(t, err)

			sts, err := BuildStatefulSet(
				t.Context(), client, es, ns, cfg,
				nil, tt.existingSSets, false, stackconfig.PolicyConfig{}, metadata.Metadata{}, "", false,
			)
			require.NoError(t, err)

			if tt.assertResources != nil {
				res, ok := elasticsearchContainerResources(sts.Spec.Template)
				require.True(t, ok, "elasticsearch container not found in pod template")
				tt.assertResources(t, res)
			}

			if tt.wantVCT != "" {
				vct := findVCT(sts.Spec.VolumeClaimTemplates, tt.wantVCT)
				require.NotNilf(t, vct, "VCT %q not found; got %v", tt.wantVCT, sts.Spec.VolumeClaimTemplates)
				requireQuantityEqual(t, vct.Spec.Resources.Requests, corev1.ResourceStorage, tt.wantStorage)
			}
		})
	}
}
