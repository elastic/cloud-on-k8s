// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
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
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("1500m")),
						Memory: ptr.To(resource.MustParse("4Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("2")),
						Memory: ptr.To(resource.MustParse("4Gi")),
					},
				},
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
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("2")),
						Memory: ptr.To(resource.MustParse("8Gi")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU:    ptr.To(resource.MustParse("2")),
						Memory: ptr.To(resource.MustParse("8Gi")),
					},
				},
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
				Resources: commonv1.Resources{
					Limits: commonv1.ResourceAllocations{
						CPU: ptr.To(resource.MustParse("1500m")),
					},
				},
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
			name: "defaults_plus_partial_nodeset_override",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						CPU: ptr.To(resource.MustParse("250m")),
					},
					Limits: commonv1.ResourceAllocations{
						CPU: ptr.To(resource.MustParse("1")),
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceCPU, "250m")
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "2Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceCPU, "1")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "2Gi")
			},
		},
		{
			name: "nodeset_memory_request_only_keeps_default_memory_limit",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: commonv1.Resources{
					Requests: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("4Gi")),
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			assertResources: func(t *testing.T, got corev1.ResourceRequirements) {
				t.Helper()
				requireQuantityEqual(t, got.Requests, corev1.ResourceMemory, "4Gi")
				requireQuantityEqual(t, got.Limits, corev1.ResourceMemory, "2Gi")
				_, hasCPUReq := got.Requests[corev1.ResourceCPU]
				_, hasCPULim := got.Limits[corev1.ResourceCPU]
				require.False(t, hasCPUReq)
				require.False(t, hasCPULim)
			},
		},
		{
			name: "nodeset_override_only_memory_cpu_from_defaults",
			nodeSet: esv1.NodeSet{
				Name:        "nodeset-1",
				Count:       1,
				Config:      &commonv1.Config{Data: map[string]any{"node.roles": []string{"master", "data"}}},
				PodTemplate: basePodTemplate(esContainerMinimal),
				Resources: commonv1.Resources{
					Limits: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("6Gi")),
					},
					Requests: commonv1.ResourceAllocations{
						Memory: ptr.To(resource.MustParse("6Gi")),
					},
				},
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
				context.Background(), client, es, nodeSet, cfg,
				nil, false, PolicyConfig{}, metadata.Metadata{}, "", false,
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
		Resources: commonv1.Resources{
			Requests: commonv1.ResourceAllocations{
				CPU: ptr.To(resource.MustParse("1")),
			},
			Limits: commonv1.ResourceAllocations{
				CPU: ptr.To(resource.MustParse("2")),
			},
		},
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
		context.Background(), client, es, nodeSet, cfg,
		nil, false, PolicyConfig{}, metadata.Metadata{}, "", false,
	)
	require.NoError(t, err)

	require.Equal(t, snapshot, DefaultResources)
}

func TestNodeSetResources_BuildStatefulSet_elasticsearch_container(t *testing.T) {
	nodeSet := esv1.NodeSet{
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
		Resources: commonv1.Resources{
			Requests: commonv1.ResourceAllocations{
				CPU:    ptr.To(resource.MustParse("1")),
				Memory: ptr.To(resource.MustParse("4Gi")),
			},
			Limits: commonv1.ResourceAllocations{
				CPU:    ptr.To(resource.MustParse("2")),
				Memory: ptr.To(resource.MustParse("4Gi")),
			},
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
	}
	es := testElasticsearchForNodeSet(nodeSet)
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
		context.Background(), client, es, ns, cfg,
		nil, nil, false, PolicyConfig{}, metadata.Metadata{}, "", false,
	)
	require.NoError(t, err)

	res, ok := elasticsearchContainerResources(sts.Spec.Template)
	require.True(t, ok)
	requireQuantityEqual(t, res.Requests, corev1.ResourceCPU, "1")
	requireQuantityEqual(t, res.Requests, corev1.ResourceMemory, "4Gi")
	requireQuantityEqual(t, res.Limits, corev1.ResourceCPU, "2")
	requireQuantityEqual(t, res.Limits, corev1.ResourceMemory, "4Gi")
}

func TestNodeSetResources_BuildStatefulSet_nil_existing_statefulsets(t *testing.T) {
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
		Resources: commonv1.Resources{
			Limits: commonv1.ResourceAllocations{
				Memory: ptr.To(resource.MustParse("3Gi")),
			},
			Requests: commonv1.ResourceAllocations{
				Memory: ptr.To(resource.MustParse("3Gi")),
			},
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
	}
	es := testElasticsearchForNodeSet(nodeSet)
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

	var existing es_sset.StatefulSetList
	sts, err := BuildStatefulSet(
		context.Background(), client, es, ns, cfg,
		nil, existing, false, PolicyConfig{}, metadata.Metadata{}, "", false,
	)
	require.NoError(t, err)

	res, ok := elasticsearchContainerResources(sts.Spec.Template)
	require.True(t, ok)
	requireQuantityEqual(t, res.Requests, corev1.ResourceMemory, "3Gi")
	requireQuantityEqual(t, res.Limits, corev1.ResourceMemory, "3Gi")
}
