// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"sync"
	"testing"

	"github.com/go-test/deep"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var onceDone = &sync.Once{}

func init() {
	onceDone.Do(func() {})
}

func TestHandleUpscaleAndSpecChanges(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	k8sClient := k8s.WrapClient(fake.NewFakeClient())
	es := v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	ctx := upscaleCtx{
		k8sClient:           k8sClient,
		es:                  es,
		scheme:              scheme.Scheme,
		observedState:       observer.State{},
		esState:             nil,
		upscaleStateBuilder: &upscaleStateBuilder{},
	}
	expectedResources := nodespec.ResourcesList{
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: common.Int32(3),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							Partition: common.Int32(3),
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
					Replicas: common.Int32(4),
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
						RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
							Partition: common.Int32(4),
						},
					},
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

	// when no StatefulSets already exists
	actualStatefulSets := sset.StatefulSetList{}
	err := HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	// StatefulSets should be created with their expected replicas
	var sset1 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset1"}, &sset1))
	require.Equal(t, common.Int32(3), sset1.Spec.Replicas)
	var sset2 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, common.Int32(4), sset2.Spec.Replicas)
	// headless services should be created for both
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset1")}, &corev1.Service{}))
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset2")}, &corev1.Service{}))
	// config should be created for both
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: name.ConfigSecret("sset1")}, &corev1.Secret{}))
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: name.ConfigSecret("sset2")}, &corev1.Secret{}))

	// upscale data nodes
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = common.Int32(10)
	err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, common.Int32(10), sset2.Spec.Replicas)

	// apply a spec change
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "b"}
	err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, "b", sset2.Spec.Template.Labels["a"])

	// apply a spec change and a downscale from 10 to 2
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = common.Int32(2)
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "c"}
	err = HandleUpscaleAndSpecChanges(ctx, actualStatefulSets, expectedResources)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	// spec should be updated
	require.Equal(t, "c", sset2.Spec.Template.Labels["a"])
	// but StatefulSet should not be downscaled
	require.Equal(t, common.Int32(10), sset2.Spec.Replicas)
}

func Test_isReplicaIncrease(t *testing.T) {
	tests := []struct {
		name     string
		actual   appsv1.StatefulSet
		expected appsv1.StatefulSet
		want     bool
	}{
		{
			name:     "increase",
			actual:   sset.TestSset{Replicas: 3}.Build(),
			expected: sset.TestSset{Replicas: 5}.Build(),
			want:     true,
		},
		{
			name:     "decrease",
			actual:   sset.TestSset{Replicas: 5}.Build(),
			expected: sset.TestSset{Replicas: 3}.Build(),
			want:     false,
		},
		{
			name:     "same value",
			actual:   sset.TestSset{Replicas: 3}.Build(),
			expected: sset.TestSset{Replicas: 3}.Build(),
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReplicaIncrease(tt.actual, tt.expected); got != tt.want {
				t.Errorf("isReplicaIncrease() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_adjustStatefulSetReplicas(t *testing.T) {
	type args struct {
		ctx                upscaleCtx
		actualStatefulSets sset.StatefulSetList
		expected           appsv1.StatefulSet
	}
	tests := []struct {
		name string
		args args
		want appsv1.StatefulSet
	}{
		{
			name: "new StatefulSet to create",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: false},
					},
				},
				actualStatefulSets: sset.StatefulSetList{},
				expected:           sset.TestSset{Name: "new-sset", Replicas: 3}.Build(),
			},
			want: sset.TestSset{Name: "new-sset", Replicas: 3}.Build(),
		},
		{
			name: "same StatefulSet already exists",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: false},
					},
				},
				actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 3}.Build(),
			},
			want: sset.TestSset{Name: "sset", Replicas: 3}.Build(),
		},
		{
			name: "downscale case",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: false},
					},
				},
				actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 1}.Build(),
			},
			want: sset.TestSset{Name: "sset", Replicas: 3}.Build(),
		},
		{
			name: "upscale case: data nodes",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: false},
					},
				},
				actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: false, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 5, Master: false, Data: true}.Build(),
			},
			want: sset.TestSset{Name: "sset", Replicas: 5, Master: false, Data: true}.Build(),
		},
		{
			name: "upscale case: master nodes - one by one",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: true},
					},
				},
				actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: true, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset", Replicas: 5, Master: true, Data: true}.Build(),
			},
			want: sset.TestSset{Name: "sset", Replicas: 4, Master: true, Data: true}.Build(),
		},
		{
			name: "upscale case: new additional master sset - one by one",
			args: args{
				ctx: upscaleCtx{
					upscaleStateBuilder: &upscaleStateBuilder{
						once:         onceDone,
						upscaleState: &upscaleState{isBootstrapped: true, allowMasterCreation: true},
					},
				},
				actualStatefulSets: sset.StatefulSetList{sset.TestSset{Name: "sset", Replicas: 3, Master: true, Data: true}.Build()},
				expected:           sset.TestSset{Name: "sset-2", Replicas: 3, Master: true, Data: true}.Build(),
			},
			want: sset.TestSset{Name: "sset-2", Replicas: 1, Master: true, Data: true}.Build(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := adjustStatefulSetReplicas(tt.args.ctx, tt.args.actualStatefulSets, tt.args.expected)
			require.NoError(t, err)
			require.Nil(t, deep.Equal(got, tt.want))
		})
	}
}

func Test_adjustZenConfig(t *testing.T) {
	bootstrappedES := v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ClusterUUIDAnnotationName: "uuid"}}}
	notBootstrappedES := v1alpha1.Elasticsearch{}

	tests := []struct {
		name                      string
		es                        v1alpha1.Elasticsearch
		resources                 nodespec.ResourcesList
		wantMinimumMasterNodesSet bool
		wantInitialMasterNodesSet bool
	}{
		{
			name: "adjust zen1 minimum_master_nodes",
			es:   bootstrappedES,
			resources: nodespec.ResourcesList{
				{
					StatefulSet: sset.TestSset{Version: "6.8.0", Replicas: 3, Master: true, Data: true}.Build(),
					Config:      settings.NewCanonicalConfig(),
				},
			},
			wantMinimumMasterNodesSet: true,
			wantInitialMasterNodesSet: false,
		},
		{
			name: "adjust zen2 initial master nodes when cluster is not bootstrapped yet",
			es:   notBootstrappedES,
			resources: nodespec.ResourcesList{
				{
					StatefulSet: sset.TestSset{Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build(),
					Config:      settings.NewCanonicalConfig(),
				},
			},
			wantMinimumMasterNodesSet: false,
			wantInitialMasterNodesSet: true,
		},
		{
			name: "don't adjust zen2 initial master nodes when cluster is already bootstrapped",
			es:   bootstrappedES,
			resources: nodespec.ResourcesList{
				{
					StatefulSet: sset.TestSset{Version: "7.2.0", Replicas: 3, Master: true, Data: true}.Build(),
					Config:      settings.NewCanonicalConfig(),
				},
			},
			wantMinimumMasterNodesSet: false,
			wantInitialMasterNodesSet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adjustZenConfig(tt.es, tt.resources)
			require.NoError(t, err)
			for _, res := range tt.resources {
				hasMinimumMasterNodes := len(res.Config.HasKeys([]string{settings.DiscoveryZenMinimumMasterNodes})) > 0
				require.Equal(t, tt.wantMinimumMasterNodesSet, hasMinimumMasterNodes)
				hasInitialMasterNodes := len(res.Config.HasKeys([]string{settings.ClusterInitialMasterNodes})) > 0
				require.Equal(t, tt.wantInitialMasterNodesSet, hasInitialMasterNodes)
			}
		})
	}
}
