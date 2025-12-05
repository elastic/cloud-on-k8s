// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/bootstrap"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

var onceDone = &sync.Once{}

func init() {
	onceDone.Do(func() {})
}

func Test_podsToCreate(t *testing.T) {
	type args struct {
		actualStatefulSets   es_sset.StatefulSetList
		expectedStatefulSets es_sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "StatefulSet does not exist yet",
			args: args{
				actualStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts1"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](5)}},
				},
				expectedStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts1"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](8)}},
					{ObjectMeta: metav1.ObjectMeta{Name: "sts2"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](2)}},
				},
			},
			want: []string{"sts1-5", "sts1-6", "sts1-7", "sts2-0", "sts2-1"},
		},
		{
			name: "StatefulSet with no replica",
			args: args{
				actualStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts1"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](5)}},
				},
				expectedStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts1"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](0)}},
					{ObjectMeta: metav1.ObjectMeta{Name: "sts2"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](2)}},
				},
			},
			want: []string{"sts2-0", "sts2-1"},
		},
		{
			name: "StatefulSet removed",
			args: args{
				actualStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts1"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](5)}},
				},
				expectedStatefulSets: []appsv1.StatefulSet{
					{ObjectMeta: metav1.ObjectMeta{Name: "sts2"}, Spec: appsv1.StatefulSetSpec{Replicas: ptr.To[int32](2)}},
				},
			},
			want: []string{"sts2-0", "sts2-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podsToCreate(tt.args.actualStatefulSets, tt.args.expectedStatefulSets)
			sort.Strings(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("podsToCreate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandleUpscaleAndSpecChanges(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
		Spec:       esv1.ElasticsearchSpec{Version: "7.5.0"},
	}
	k8sClient := k8s.NewFakeClient(&es)
	ctx := upscaleCtx{
		k8sClient:    k8sClient,
		es:           es,
		esState:      nil,
		expectations: expectations.NewExpectations(k8sClient),
		parentCtx:    context.Background(),
	}
	expectedResources := nodespec.ResourcesList{
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](3),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								string(label.NodeTypesMasterLabelName): "true",
							},
						},
					},
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
			},
			Config: settings.CanonicalConfig{},
		},
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset2",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](4),
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset2",
				},
			},
			Config: settings.CanonicalConfig{},
		},
	}

	expectedResources[0].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResources[0].StatefulSet.Labels, expectedResources[0].StatefulSet.Spec)
	expectedResources[1].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResources[1].StatefulSet.Labels, expectedResources[1].StatefulSet.Spec)

	// when no StatefulSets already exists
	actualStatefulSets := es_sset.StatefulSetList{}
	res, err := HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	// StatefulSets should be created with their expected replicas
	var sset1 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sset1"}, &sset1))
	require.Equal(t, ptr.To[int32](3), sset1.Spec.Replicas)
	comparison.RequireEqual(t, &res.ActualStatefulSets[0], &sset1)
	var sset2 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, ptr.To[int32](4), sset2.Spec.Replicas)
	comparison.RequireEqual(t, &res.ActualStatefulSets[1], &sset2)
	// headless services should be created for both
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset1")}, &corev1.Service{}))
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset2")}, &corev1.Service{}))
	// config should be created for both
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: esv1.ConfigSecret("sset1")}, &corev1.Secret{}))
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: esv1.ConfigSecret("sset2")}, &corev1.Secret{}))

	// upscale data nodes
	actualStatefulSets = es_sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = ptr.To[int32](10)
	// re-fetch es to simulate actual reconciliation behaviour
	require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es.ObjectMeta), &es))
	// update context with current version of Elasticsearch resource
	ctx.es = es
	res, err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, ptr.To[int32](10), sset2.Spec.Replicas)
	comparison.RequireEqual(t, &res.ActualStatefulSets[1], &sset2)
	// expectations should have been set
	require.NotEmpty(t, ctx.expectations.GetGenerations())
	// apply a spec change
	actualStatefulSets = es_sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "b"}
	res, err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, "b", sset2.Spec.Template.Labels["a"])
	comparison.RequireEqual(t, &res.ActualStatefulSets[1], &sset2)

	// apply a spec change and a downscale from 10 to 2
	actualStatefulSets = es_sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = ptr.To[int32](2)
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "c"}
	res, err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.False(t, res.Requeue)
	require.Len(t, res.ActualStatefulSets, 2)
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	// spec should be updated
	require.Equal(t, "c", sset2.Spec.Template.Labels["a"])
	// but StatefulSet should not be downscaled
	require.Equal(t, ptr.To[int32](10), sset2.Spec.Replicas)
	comparison.RequireEqual(t, &res.ActualStatefulSets[1], &sset2)
}

func TestHandleUpscaleAndSpecChanges_PVCResize(t *testing.T) {
	// focus on the special case of handling PVC resize
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es", Annotations: map[string]string{
			// simulate annotation already set otherwise we get a conflict when es is updated twice
			// (first for initial master nodes, then for sset recreation)
			"elasticsearch.k8s.elastic.co/initial-master-nodes": "sset1-0,sset1-1,sset1-2",
		}},
		Spec: esv1.ElasticsearchSpec{Version: "7.5.0"},
	}

	truePtr := true
	storageClass := storagev1.StorageClass{
		ObjectMeta:           metav1.ObjectMeta{Name: "resizeable"},
		AllowVolumeExpansion: &truePtr,
	}

	// 3 masters, 4 data x 1Gi storage
	actualStatefulSets := []appsv1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset1",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To[int32](3),
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							string(label.NodeTypesMasterLabelName): "true",
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "sset2",
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: ptr.To[int32](4),
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
							StorageClassName: &storageClass.Name,
						},
					},
				},
			},
		},
	}
	// expected: same 2 StatefulSets, but the 2nd one has its storage resized to 3Gi
	dataResized := *actualStatefulSets[1].DeepCopy()
	dataResized.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("3Gi")
	expectedResources := nodespec.ResourcesList{
		{
			StatefulSet: actualStatefulSets[0],
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
			},
			Config: settings.CanonicalConfig{},
		},
		{
			StatefulSet: dataResized,
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset2",
				},
			},
			Config: settings.CanonicalConfig{},
		},
	}

	k8sClient := k8s.NewFakeClient(&es, &storageClass, &actualStatefulSets[0], &actualStatefulSets[1])
	// retrieve the created es with its resource version set
	require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es.ObjectMeta), &es))
	ctx := upscaleCtx{
		k8sClient:    k8sClient,
		es:           es,
		esState:      nil,
		expectations: expectations.NewExpectations(k8sClient),
		parentCtx:    context.Background(),
	}

	// 2nd StatefulSet should be marked for recreation, we should requeue
	res, err := HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.True(t, res.Requeue)
	require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&es.ObjectMeta), &es))
	require.Len(t, es.Annotations, 2) // initial master nodes + sset to recreate
	_, hasRecreateAnnotationForSset2 := es.Annotations["elasticsearch.k8s.elastic.co/recreate-sset2"]
	require.True(t, hasRecreateAnnotationForSset2)
}

func Test_adjustStatefulSetReplicas(t *testing.T) {
	type args struct {
		state              *upscaleState
		actualStatefulSets es_sset.StatefulSetList
		expected           appsv1.StatefulSet
	}
	tests := []struct {
		name             string
		args             args
		want             appsv1.StatefulSet
		wantUpscaleState *upscaleState
	}{
		{
			name: "new StatefulSet to create",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{},
				expected:           sset.TestSset{Name: "new-sset", Replicas: 3}.Build(),
			},
			want:             sset.TestSset{Name: "new-sset", Replicas: 3}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 3, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
		{
			name: "same StatefulSet already exists",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 3}.Build(),
			},
			want:             sset.TestSset{Name: "sset", Replicas: 3}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 0, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
		{
			name: "downscale case",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 1}.Build(),
			},
			want:             sset.TestSset{Name: "sset", Replicas: 3}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 0, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
		{
			name: "upscale case: data nodes",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: false, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 5, Master: false, Data: true}.Build(),
			},
			want:             sset.TestSset{Name: "sset", Replicas: 5, Master: false, Data: true}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 2, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
		{
			name: "upscale case: master nodes - one by one",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: true, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: true, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 5, Master: true, Data: true}.Build(),
			},
			want:             sset.TestSset{Name: "sset", Replicas: 4, Master: true, Data: true}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 1, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
		{
			name: "upscale case: new additional master sset - one by one",
			args: args{
				state:              &upscaleState{isBootstrapped: true, allowMasterCreation: true, createsAllowed: ptr.To[int32](3)},
				actualStatefulSets: es_sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: true, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset-2", Replicas: 3, Master: true, Data: true}.Build(),
			},
			want:             sset.TestSset{Name: "sset-2", Replicas: 1, Master: true, Data: true}.Build(),
			wantUpscaleState: &upscaleState{recordedCreates: 1, isBootstrapped: true, allowMasterCreation: false, createsAllowed: ptr.To[int32](3)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adjustStatefulSetReplicas(tt.args.state, tt.args.actualStatefulSets, tt.args.expected)
			require.NoError(t, err)
			require.Nil(t, deep.Equal(got, tt.want))
			require.Equal(t, tt.wantUpscaleState, tt.args.state)
		})
	}
}

func Test_adjustZenConfig(t *testing.T) {
	bootstrappedES := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:        TestEsName,
			Namespace:   TestEsNamespace,
			Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"},
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.5.0"},
	}
	notBootstrappedES := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestEsName,
			Namespace: TestEsNamespace,
		},
		Spec: esv1.ElasticsearchSpec{Version: "7.5.0"},
	}

	tests := []struct {
		name                      string
		es                        esv1.Elasticsearch
		statefulSet               sset.TestSset
		pods                      []client.Object
		wantMinimumMasterNodesSet bool
		wantInitialMasterNodesSet bool
	}{
		{
			name:                      "adjust zen1 minimum_master_nodes",
			es:                        bootstrappedES,
			statefulSet:               sset.TestSset{Version: "6.8.0", Replicas: 3, Master: true, Data: true},
			wantMinimumMasterNodesSet: true,
			wantInitialMasterNodesSet: false,
		},
		{
			name:        "adjust zen1 minimum_master_nodes if some 6.8.x are still in flight",
			es:          bootstrappedES,
			statefulSet: sset.TestSset{Name: "masters", Version: "7.2.0", Replicas: 3, Master: true, Data: true},
			pods: []client.Object{
				newTestPod("masters-0").withVersion("6.8.0").withRoles(esv1.MasterRole, esv1.DataRole).toPodPtr(),
				newTestPod("masters-1").withVersion("6.8.0").withRoles(esv1.MasterRole, esv1.DataRole).toPodPtr(),
				newTestPod("masters-2").withVersion("6.8.0").withRoles(esv1.MasterRole, esv1.DataRole).toPodPtr(),
			},
			wantMinimumMasterNodesSet: true,
			wantInitialMasterNodesSet: false,
		},
		{
			name:                      "adjust zen2 initial master nodes when cluster is not bootstrapped yet",
			es:                        notBootstrappedES,
			statefulSet:               sset.TestSset{Version: "7.2.0", Replicas: 3, Master: true, Data: true},
			wantMinimumMasterNodesSet: false,
			wantInitialMasterNodesSet: true,
		},
		{
			name:                      "don't adjust zen2 initial master nodes when cluster is already bootstrapped",
			es:                        bootstrappedES,
			statefulSet:               sset.TestSset{Version: "7.2.0", Replicas: 3, Master: true, Data: true},
			wantMinimumMasterNodesSet: false,
			wantInitialMasterNodesSet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources := nodespec.ResourcesList{
				{
					StatefulSet: tt.statefulSet.Build(),
					Config:      settings.NewCanonicalConfig(),
				},
			}
			pods := tt.pods
			if pods == nil {
				pods = tt.statefulSet.Pods()
			}
			client := k8s.NewFakeClient(append(pods, &tt.es)...)
			err := adjustZenConfig(context.Background(), client, tt.es, resources)
			require.NoError(t, err)
			for _, res := range resources {
				hasMinimumMasterNodes := len(res.Config.HasKeys([]string{esv1.DiscoveryZenMinimumMasterNodes})) > 0
				require.Equal(t, tt.wantMinimumMasterNodesSet, hasMinimumMasterNodes)
				hasInitialMasterNodes := len(res.Config.HasKeys([]string{esv1.ClusterInitialMasterNodes})) > 0
				require.Equal(t, tt.wantInitialMasterNodesSet, hasInitialMasterNodes)
			}
		})
	}
}

func Test_adjustResources(t *testing.T) {
	type args struct {
		es                 esv1.Elasticsearch
		actualStatefulSets es_sset.StatefulSetList
		expectedResources  nodespec.ResourcesList
	}
	tests := []struct {
		name      string
		args      args
		wantSsets es_sset.StatefulSetList
	}{
		{
			name: "initial cluster creation: add all masters from several nodeSets",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"},
					Spec:       esv1.ElasticsearchSpec{Version: "7.5.0"},
				},
				actualStatefulSets: nil,
				expectedResources: nodespec.ResourcesList{
					{
						StatefulSet: sset.TestSset{Name: "masters1", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
						Config:      settings.NewCanonicalConfig(),
					},
					{
						StatefulSet: sset.TestSset{Name: "masters2", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
						Config:      settings.NewCanonicalConfig(),
					},
				},
			},
			wantSsets: es_sset.StatefulSetList{
				sset.TestSset{Name: "masters1", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
				sset.TestSset{Name: "masters2", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
			},
		},
		{
			name: "cluster already bootstrapped: add masters one by one",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns", Annotations: map[string]string{bootstrap.ClusterUUIDAnnotationName: "uuid"}},
					Spec:       esv1.ElasticsearchSpec{Version: "7.5.0"},
				},
				actualStatefulSets: nil,
				expectedResources: nodespec.ResourcesList{
					{
						StatefulSet: sset.TestSset{Name: "masters1", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
						Config:      settings.NewCanonicalConfig(),
					},
					{
						StatefulSet: sset.TestSset{Name: "masters2", Master: true, Replicas: 3, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
						Config:      settings.NewCanonicalConfig(),
					},
				},
			},
			wantSsets: es_sset.StatefulSetList{
				sset.TestSset{Name: "masters1", Master: true, Replicas: 1, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
				sset.TestSset{Name: "masters2", Master: true, Replicas: 0, Namespace: "ns", ClusterName: "es", Version: "7.5.0"}.Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(&tt.args.es)
			ctx := upscaleCtx{
				parentCtx:    context.Background(),
				es:           tt.args.es,
				k8sClient:    k8sClient,
				expectations: expectations.NewExpectations(k8sClient),
			}
			got, err := adjustResources(ctx, tt.args.actualStatefulSets, tt.args.expectedResources)
			require.NoError(t, err)
			require.Nil(t, deep.Equal(got.StatefulSets(), tt.wantSsets))
		})
	}
}

func TestHandleUpscaleAndSpecChanges_VersionUpgradeDataFirstFlow(t *testing.T) {
	// Test the complete upgrade flow: data nodes upgrade first, then master nodes
	// starting at 8.16.2 and upgrading to 8.17.1
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
			Annotations: map[string]string{
				"elasticsearch.k8s.elastic.co/initial-master-nodes": "node-1,node-2,node-3",
				bootstrap.ClusterUUIDAnnotationName:                 "uuid",
			},
		},
		Spec:   esv1.ElasticsearchSpec{Version: "8.16.2"},
		Status: esv1.ElasticsearchStatus{Version: "8.16.2"},
	}
	k8sClient := k8s.NewFakeClient(&es)
	ctx := upscaleCtx{
		k8sClient:    k8sClient,
		es:           es,
		esState:      nil,
		expectations: expectations.NewExpectations(k8sClient),
		parentCtx:    context.Background(),
	}

	// Create expected resources with both master and data StatefulSets at 8.16.2
	expectedResources := nodespec.ResourcesList{
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "master-sset",
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/node-master": "true",
						"elasticsearch.k8s.elastic.co/version":     "8.16.2",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](3),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"elasticsearch.k8s.elastic.co/node-master": "true",
								"elasticsearch.k8s.elastic.co/version":     "8.16.2",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "elasticsearch",
									Image: "docker.elastic.co/elasticsearch/elasticsearch:8.16.2",
								},
							},
						},
					},
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "master-sset",
				},
			},
			Config: settings.NewCanonicalConfig(),
		},
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "data-sset",
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/node-data": "true",
						"elasticsearch.k8s.elastic.co/version":   "8.16.2",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](2),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"elasticsearch.k8s.elastic.co/node-data": "true",
								"elasticsearch.k8s.elastic.co/version":   "8.16.2",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "elasticsearch",
									Image: "docker.elastic.co/elasticsearch/elasticsearch:8.16.2",
								},
							},
						},
					},
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "data-sset",
				},
			},
			Config: settings.NewCanonicalConfig(),
		},
	}

	// Set template hash labels
	expectedResources[0].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResources[0].StatefulSet.Labels, expectedResources[0].StatefulSet.Spec)
	expectedResources[1].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResources[1].StatefulSet.Labels, expectedResources[1].StatefulSet.Spec)

	// Call HandleUpscaleAndSpecChanges and check things are created properly
	actualStatefulSets := es_sset.StatefulSetList{}
	res, err := HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.Len(t, res.ActualStatefulSets, 2)

	// Verify both StatefulSets were created at 8.16.2
	var masterSset appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "master-sset"}, &masterSset))
	require.NotNil(t, masterSset.Spec.Replicas)
	// Master nodes/pods are limited to 1 creation at a time regardless of the replicas setting.
	require.Equal(t, int32(1), *masterSset.Spec.Replicas)
	require.Equal(t, "docker.elastic.co/elasticsearch/elasticsearch:8.16.2", masterSset.Spec.Template.Spec.Containers[0].Image)

	// Set master StatefulSet status to show it's fully deployed at 8.16.2
	// Also update the replicas to 3 to simulate full rollout at 8.16.2
	masterSset.Spec.Replicas = ptr.To[int32](3)
	masterSset.Status.Replicas = 3
	masterSset.Status.UpdatedReplicas = 3
	masterSset.Status.CurrentRevision = "master-sset-old"
	masterSset.Status.UpdateRevision = "master-sset-old"
	require.NoError(t, k8sClient.Update(context.Background(), &masterSset))
	require.NoError(t, k8sClient.Status().Update(context.Background(), &masterSset))

	var dataSset appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "data-sset"}, &dataSset))
	require.NotNil(t, dataSset.Spec.Replicas)
	require.Equal(t, int32(2), *dataSset.Spec.Replicas)
	require.Equal(t, "docker.elastic.co/elasticsearch/elasticsearch:8.16.2", dataSset.Spec.Template.Spec.Containers[0].Image)

	// Set data StatefulSet status to show it's fully deployed at 8.16.2
	dataSset.Status.Replicas = 2
	dataSset.Status.UpdatedReplicas = 2
	dataSset.Status.CurrentRevision = "data-sset-old"
	dataSset.Status.UpdateRevision = "data-sset-old"
	require.NoError(t, k8sClient.Status().Update(context.Background(), &dataSset))

	// Create pods for both StatefulSets with the old revision
	masterPods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "master-sset-0",
				Namespace: "ns",
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/node-master": "true",
					"controller-revision-hash":                 "master-sset-old",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "master-sset-1",
				Namespace: "ns",
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/node-master": "true",
					"controller-revision-hash":                 "master-sset-old",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "master-sset-2",
				Namespace: "ns",
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/node-master": "true",
					"controller-revision-hash":                 "master-sset-old",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
	}
	for _, pod := range masterPods {
		require.NoError(t, k8sClient.Create(context.Background(), &pod))
	}

	dataPods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data-sset-0",
				Namespace: "ns",
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/node-data": "true",
					"controller-revision-hash":               "data-sset-old",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data-sset-1",
				Namespace: "ns",
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/node-data": "true",
					"controller-revision-hash":               "data-sset-old",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
	}
	for _, pod := range dataPods {
		require.NoError(t, k8sClient.Create(context.Background(), &pod))
	}

	// Update the ES object to 8.17.1 in k8s
	es.Spec.Version = "8.17.1"
	require.NoError(t, k8sClient.Update(context.Background(), &es))
	ctx.es = es

	// Update actualStatefulSets to reflect the current state with status
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "master-sset"}, &masterSset))
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "data-sset"}, &dataSset))
	actualStatefulSets = es_sset.StatefulSetList{masterSset, dataSset}

	// Update expected resources to 8.17.1 for the upgrade
	expectedResourcesUpgrade := nodespec.ResourcesList{
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "master-sset",
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/node-master": "true",
						"elasticsearch.k8s.elastic.co/version":     "8.17.1",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](4), // also upscale the master replicas to ensure that an upscale during an upgrade can happen in parallel with non-masters.
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"elasticsearch.k8s.elastic.co/node-master": "true",
								"elasticsearch.k8s.elastic.co/version":     "8.17.1",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "elasticsearch",
									Image: "docker.elastic.co/elasticsearch/elasticsearch:8.17.1",
								},
							},
						},
					},
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "master-sset",
				},
			},
			Config: settings.NewCanonicalConfig(),
		},
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "data-sset",
					Labels: map[string]string{
						"elasticsearch.k8s.elastic.co/node-data": "true",
						"elasticsearch.k8s.elastic.co/version":   "8.17.1",
					},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr.To[int32](2),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"elasticsearch.k8s.elastic.co/node-data": "true",
								"elasticsearch.k8s.elastic.co/version":   "8.17.1",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "elasticsearch",
									Image: "docker.elastic.co/elasticsearch/elasticsearch:8.17.1",
								},
							},
						},
					},
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "data-sset",
				},
			},
			Config: settings.NewCanonicalConfig(),
		},
	}

	// Set template hash labels for upgrade resources
	expectedResourcesUpgrade[0].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResourcesUpgrade[0].StatefulSet.Labels, expectedResourcesUpgrade[0].StatefulSet.Spec)
	expectedResourcesUpgrade[1].StatefulSet.Labels = hash.SetTemplateHashLabel(expectedResourcesUpgrade[1].StatefulSet.Labels, expectedResourcesUpgrade[1].StatefulSet.Spec)

	// Manually set the data StatefulSet status to show it's NOT fully upgraded
	// This simulates the state after the StatefulSet controller has updated the spec but before the pods are updated
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "data-sset"}, &dataSset))
	dataSset.Status.UpdatedReplicas = 0                // No replicas updated yet
	dataSset.Status.Replicas = 2                       // Total replicas
	dataSset.Status.UpdateRevision = "data-sset-12345" // New revision (different from old)
	require.NoError(t, k8sClient.Status().Update(context.Background(), &dataSset))

	// Call HandleUpscaleAndSpecChanges and verify that both data upgrade has begun and master STS is not updated
	_, err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResourcesUpgrade)
	require.NoError(t, err)

	// Verify data StatefulSet is updated to 8.17.1
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "data-sset"}, &dataSset))
	require.Equal(t, "docker.elastic.co/elasticsearch/elasticsearch:8.17.1", dataSset.Spec.Template.Spec.Containers[0].Image)

	// Verify master StatefulSet version hasn't changed yet (should still be 8.16.2)
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "master-sset"}, &masterSset))
	require.Equal(t, "docker.elastic.co/elasticsearch/elasticsearch:8.16.2", masterSset.Spec.Template.Spec.Containers[0].Image)
	// Verify master StatefulSet replicas have been scaled up to 4
	require.Equal(t, int32(4), *masterSset.Spec.Replicas)

	// Update data STS and associated pods to show they are completely upgraded
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "data-sset"}, &dataSset))
	dataSset.Status.UpdatedReplicas = 2                // All replicas updated
	dataSset.Status.Replicas = 2                       // Total replicas
	dataSset.Status.UpdateRevision = "data-sset-12345" // Set update revision
	require.NoError(t, k8sClient.Status().Update(context.Background(), &dataSset))

	// Update the existing data pods to have the new revision
	for i := 0; i < int(dataSset.Status.Replicas); i++ {
		var pod corev1.Pod
		require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: fmt.Sprintf("data-sset-%d", i)}, &pod))
		pod.Labels["controller-revision-hash"] = "data-sset-12345"
		require.NoError(t, k8sClient.Update(context.Background(), &pod))
	}
	// Call HandleUpscaleAndSpecChanges and verify that master STS is now set to be upgraded
	actualStatefulSets = res.ActualStatefulSets
	_, err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResourcesUpgrade)
	require.NoError(t, err)

	// Verify master StatefulSet is now updated to 8.17.1
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "master-sset"}, &masterSset))
	require.Equal(t, "docker.elastic.co/elasticsearch/elasticsearch:8.17.1", masterSset.Spec.Template.Spec.Containers[0].Image)
}
