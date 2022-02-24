// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func podWithRevision(name, revision string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestEsNamespace,
			Labels:    map[string]string{appsv1.StatefulSetRevisionLabel: revision},
		},
	}
}

func Test_podsToUpgrade(t *testing.T) {
	type args struct {
		pods         []runtime.Object
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "all pods need to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 3},
					}.Build(),
				},
				pods: []runtime.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
					podWithRevision("nodes-0", "rev-a"),
					podWithRevision("nodes-1", "rev-a"),
					podWithRevision("nodes-2", "rev-a"),
				},
			},
			want: []string{"masters-0", "masters-1", "nodes-0", "nodes-1", "nodes-2"},
		},
		{
			name: "only a sset needs to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []runtime.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{"masters-0", "masters-1"},
		},
		{
			name: "no pods to upgrade if the StatefulSet UpdateRevision is empty",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []runtime.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{},
		},
		{
			name: "only 1 node need to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 1, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []runtime.Object{
					podWithRevision("masters-0", "rev-b"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{"masters-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.args.pods...)
			got, err := podsToUpgrade(client, tt.args.statefulSets)
			if (err != nil) != tt.wantErr {
				t.Errorf("podsToUpgrade() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.ElementsMatch(t, names(got), tt.want, tt.name)
		})
	}
}

func Test_healthyPods(t *testing.T) {
	type args struct {
		pods         upgradeTestPods
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "All Pods are healthy",
			args: args{
				pods: newUpgradeTestPods(
					newTestPod("masters-2").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
					newTestPod("masters-1").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
					newTestPod("masters-0").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
				),
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name:      "masters",
						Namespace: TestEsNamespace,
						Replicas:  3,
					}.Build(),
				},
			},
		},
		{
			name: "One Pod is terminating",
			args: args{
				pods: newUpgradeTestPods(
					newTestPod("masters-2").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
					newTestPod("masters-1").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).isTerminating(true).withResourceVersion("999"),
					newTestPod("masters-0").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
				),
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name:      "masters",
						Namespace: TestEsNamespace,
						Replicas:  3,
					}.Build(),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esState := &testESState{
				inCluster: tt.args.pods.podsInCluster(),
			}
			client := k8s.NewFakeClient(tt.args.pods.toRuntimeObjects("7.5.0", 0, nothing, nil)...)
			got, err := healthyPods(client, tt.args.statefulSets, esState)
			if (err != nil) != tt.wantErr {
				t.Errorf("healthyPods() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			want := tt.args.pods.toHealthyPods()
			assert.Equal(t, len(want), len(got))
			assert.Equal(t, want, got)
		})
	}
}

func Test_doFlush(t *testing.T) {
	tests := []struct {
		name                string
		es                  esv1.Elasticsearch
		wantSyncFlushCalled bool
		wantFlushCalled     bool
	}{
		{
			name:                "flush when target version is 8.x",
			es:                  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "8.0.0"}},
			wantFlushCalled:     true,
			wantSyncFlushCalled: false,
		},
		{
			name:                "sync flush when target version is below 8.x",
			es:                  esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{Version: "7.6.0"}},
			wantFlushCalled:     false,
			wantSyncFlushCalled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := &fakeESClient{}
			err := doFlush(context.Background(), tt.es, fakeClient)
			require.NoError(t, err)
			require.Equal(t, tt.wantSyncFlushCalled, fakeClient.SyncedFlushCalled)
			require.Equal(t, tt.wantFlushCalled, fakeClient.FlushCalled)
		})
	}
}

func Test_isNonHACluster(t *testing.T) {
	type args struct {
		actualPods      []corev1.Pod
		expectedMasters []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "single node cluster is not HA",
			args: args{
				actualPods: []corev1.Pod{
					sset.TestPod{Name: "pod-0", Master: true}.Build(),
				},
				expectedMasters: []string{"pod-0"},
			},
			want: true,
		},
		{
			name: "two node cluster is not HA",
			args: args{
				actualPods: []corev1.Pod{
					sset.TestPod{Name: "pod-0", Master: true}.Build(),
					sset.TestPod{Name: "pod-1", Master: true}.Build(),
				},
				expectedMasters: []string{"pod-0", "pod-1"},
			},
			want: true,
		},
		{
			name: "multi-node cluster with two masters is not HA",
			args: args{
				actualPods: []corev1.Pod{
					sset.TestPod{Name: "master-0", StatefulSetName: "masters", Master: true}.Build(),
					sset.TestPod{Name: "master-1", StatefulSetName: "masters", Master: true}.Build(),
					sset.TestPod{Name: "data-0", StatefulSetName: "data", Data: true}.Build(),
				},
				expectedMasters: []string{"pod-0", "pod-1"},
			},
			want: true,
		},
		{
			name: "more than two master nodes is HA",
			args: args{
				actualPods: []corev1.Pod{
					sset.TestPod{Name: "pod-0", Master: true}.Build(),
					sset.TestPod{Name: "pod-1", Master: true}.Build(),
					sset.TestPod{Name: "pod-2", Master: true}.Build(),
				},
				expectedMasters: []string{"pod-0", "pod-1", "pod-2"},
			},
			want: false,
		},
		{
			name: "more than two master nodes but only two rolled out should be considered HA",
			args: args{
				actualPods: []corev1.Pod{
					sset.TestPod{Name: "pod-0", Master: true}.Build(),
					sset.TestPod{Name: "pod-1", Master: true}.Build(),
				},
				expectedMasters: []string{"pod-0", "pod-1", "pod-2"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, isNonHACluster(tt.args.actualPods, tt.args.expectedMasters), "isNonHACluster(%v, %v)", tt.args.actualPods, tt.args.expectedMasters)
		})
	}
}

func Test_isVersionUpgrade(t *testing.T) {
	tests := []struct {
		name    string
		es      esv1.Elasticsearch
		want    bool
		wantErr bool
	}{
		{
			name: "upgrade",
			es: esv1.Elasticsearch{
				Spec:   esv1.ElasticsearchSpec{Version: "8.0.0"},
				Status: esv1.ElasticsearchStatus{Version: "7.17.0"},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "minor upgrade",
			es: esv1.Elasticsearch{
				Spec:   esv1.ElasticsearchSpec{Version: "8.1.0"},
				Status: esv1.ElasticsearchStatus{Version: "8.0.0"},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "not an upgrade",
			es: esv1.Elasticsearch{
				Spec:   esv1.ElasticsearchSpec{Version: "7.17.0"},
				Status: esv1.ElasticsearchStatus{Version: "7.17.0"},
			},
			want:    false,
			wantErr: false,
		},

		{
			name: "corrupted status version",
			es: esv1.Elasticsearch{
				Spec:   esv1.ElasticsearchSpec{Version: "7.17.0"},
				Status: esv1.ElasticsearchStatus{Version: "NaV"},
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "corrupted spec version",
			es: esv1.Elasticsearch{
				Spec:   esv1.ElasticsearchSpec{Version: "should never happen"},
				Status: esv1.ElasticsearchStatus{Version: "7.17.0"},
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := isVersionUpgrade(tt.es)
			if tt.wantErr != (err != nil) {
				t.Errorf("wantErr %v got %v", tt.wantErr, err)
			}
			assert.Equalf(t, tt.want, got, "isVersionUpgrade(%v)", tt.es)
		})
	}
}
