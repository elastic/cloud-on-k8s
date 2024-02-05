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
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
		pods         []client.Object
		statefulSets es_sset.StatefulSetList
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
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 3},
					}.Build(),
				},
				pods: []client.Object{
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
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []client.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{"masters-0", "masters-1"},
		},
		{
			name: "no pods to upgrade if the StatefulSet UpdateRevision is empty",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []client.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{},
		},
		{
			name: "only 1 node need to be upgraded",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Namespace: TestEsNamespace, Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 1, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Namespace: TestEsNamespace, Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: []client.Object{
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
		statefulSets es_sset.StatefulSetList
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
				statefulSets: es_sset.StatefulSetList{
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
					newTestPod("masters-1").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).
						isTerminating(true).withResourceVersion("999").withFinalizers([]string{"something"}),
					newTestPod("masters-0").inStatefulset("masters").withRoles(esv1.MasterRole).isHealthy(true).needsUpgrade(true).isInCluster(true).withResourceVersion("999"),
				),
				statefulSets: es_sset.StatefulSetList{
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
			client := k8s.NewFakeClient(tt.args.pods.toClientObjects("7.5.0", 0, nothing, nil)...)
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

func Test_defaultDriver_maybeCompleteNodeUpgrades(t *testing.T) {
	esVersion := "8.1.0"
	clusterName = "test-cluster"
	namespace := "ns"
	es = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
	}

	testSset := sset.TestSset{
		Namespace:   namespace,
		Name:        "es",
		ClusterName: clusterName,
		Version:     esVersion,
		Replicas:    2,
	}
	shutdownFixture := map[string]esclient.NodeShutdown{
		"node-id-0": {
			NodeID: "node-id-0",
			Type:   "RESTART",
			Status: "COMPLETE",
		},
	}
	leftOverShutdownFixture := map[string]esclient.NodeShutdown{
		"node-id-99": {
			NodeID: "node-id-99",
			Type:   "RESTART",
			Status: "COMPLETE",
		},
	}
	disabledAllocationFixture := esclient.ClusterRoutingAllocation{
		Transient: esclient.AllocationSettings{
			Cluster: esclient.ClusterRoutingSettings{
				Routing: esclient.RoutingSettings{
					Allocation: esclient.RoutingAllocationSettings{
						Enable: "none",
					},
				},
			},
		},
	}
	tests := []struct {
		name              string
		es                esv1.Elasticsearch
		nodesInCluster    map[string]esclient.Node
		shutdowns         map[string]esclient.NodeShutdown
		routingAllocation esclient.ClusterRoutingAllocation
		runtimeObjects    []client.Object
		expectations      func(*expectations.Expectations)
		assertions        func(*reconciler.Results, *fakeESClient)
	}{
		{
			name: "unsatisfied expectations: no shutdown clean up",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
			},
			shutdowns:      shutdownFixture,
			runtimeObjects: append(testSset.Pods(), testSset.BuildPtr()),
			expectations: func(e *expectations.Expectations) {
				e.ExpectDeletion(sset.TestPod{Namespace: namespace, Name: "es-0", ClusterName: clusterName}.Build())
			},
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.False(t, esClient.DeleteShutdownCalled)
				require.False(t, esClient.EnableShardAllocationCalled)
				reconciled, _ := results.IsReconciled()
				require.False(t, reconciled)
			},
		},
		{
			name: "expectations satisfied: restarted node back in cluster but not all nodes",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
			},
			shutdowns:      shutdownFixture,
			runtimeObjects: append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.True(t, esClient.DeleteShutdownCalled)
				require.False(t, esClient.EnableShardAllocationCalled)
				reconciled, _ := results.IsReconciled()
				require.False(t, reconciled)
			},
		},
		{
			name: "not all nodes in cluster, routing disabled, left over shutdown: no calls",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
			},
			shutdowns:         leftOverShutdownFixture,
			routingAllocation: disabledAllocationFixture,
			runtimeObjects:    append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.False(t, esClient.DeleteShutdownCalled)
				require.False(t, esClient.EnableShardAllocationCalled)
				reconciled, _ := results.IsReconciled()
				require.False(t, reconciled)
			},
		},
		{
			name: "all nodes in cluster, left over shutdown cleaned up",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
				"node-id-1": {Name: "es-1"},
			},
			shutdowns:      leftOverShutdownFixture,
			runtimeObjects: append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.True(t, esClient.DeleteShutdownCalled)
				require.False(t, esClient.EnableShardAllocationCalled)
				reconciled, _ := results.IsReconciled()
				require.True(t, reconciled)
			},
		},
		{
			name: "all nodes in cluster, routing allocation re-enabled",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
				"node-id-1": {Name: "es-1"},
			},
			routingAllocation: disabledAllocationFixture,
			runtimeObjects:    append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.False(t, esClient.DeleteShutdownCalled)
				require.True(t, esClient.EnableShardAllocationCalled)
				reconciled, _ := results.IsReconciled()
				require.True(t, reconciled)
			},
		},
		{
			name: "all nodes in cluster, orchestration hint set: no call",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName,
					Namespace: namespace,
					Annotations: map[string]string{
						hints.OrchestrationsHintsAnnotation: `{"no_transient_settings": true}`,
					},
				},
			},
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
				"node-id-1": {Name: "es-1"},
			},
			runtimeObjects: append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.False(t, esClient.DeleteShutdownCalled)
				require.False(t, esClient.EnableShardAllocationCalled)
				require.Equal(t, 0, esClient.GetClusterRoutingAllocationCallCount)
				reconciled, _ := results.IsReconciled()
				require.True(t, reconciled)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.runtimeObjects...)
			esClient := &fakeESClient{
				version:                  version.MustParse(esVersion),
				nodes:                    esclient.Nodes{Nodes: tt.nodesInCluster},
				Shutdowns:                tt.shutdowns,
				clusterRoutingAllocation: tt.routingAllocation,
			}
			esState := NewMemoizingESState(context.Background(), esClient)

			reconcileState, err := reconcile.NewState(tt.es)
			require.NoError(t, err)

			d := &defaultDriver{
				DefaultDriverParameters: DefaultDriverParameters{
					Client:         client,
					ES:             tt.es,
					Expectations:   expectations.NewExpectations(client),
					ReconcileState: reconcileState,
				},
			}
			if tt.expectations != nil {
				tt.expectations(d.Expectations)
			}

			nodeNameToID, err := esState.NodeNameToID()
			require.NoError(t, err)

			n := shutdown.NewNodeShutdown(esClient, nodeNameToID, esclient.Restart, "", crlog.Log)
			results := d.maybeCompleteNodeUpgrades(context.Background(), esClient, esState, n)
			tt.assertions(results, esClient)
		})
	}
}
