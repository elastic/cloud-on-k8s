// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"
)

func podWithRevision(name, revision string) *corev1.Pod {
	return &corev1.Pod{
		Name:      name,
		Namespace: TestEsNamespace,
		Labels:    map[string]string{appsv1.StatefulSetRevisionLabel: revision},
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

// clusterMembers is an ESState of a cluster that only has the given members.
type clusterMembers struct {
	ESState
	members set.StringSet
}

func (c clusterMembers) NodeNameToID() (map[string]string, error) {
	nodeNameToID := make(map[string]string, len(c.members))
	for nodeName := range c.members {
		nodeNameToID[nodeName] = nodeName
	}
	return nodeNameToID, nil
}

func (c clusterMembers) NodesInCluster(nodeNames []string) (bool, error) {
	for _, nodeName := range nodeNames {
		if !c.members.Has(nodeName) {
			return false, nil
		}
	}
	return true, nil
}

func Test_podsToRollingUpgrade(t *testing.T) {
	type args struct {
		pods                 []client.Object
		statefulSets         es_sset.StatefulSetList
		expectedStatefulSets es_sset.StatefulSetList
		// members are the Pods that are members of the cluster
		members set.StringSet
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "no StatefulSet is being removed: same as podsToUpgrade",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
				},
				expectedStatefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2}.Build(),
				},
				pods: []client.Object{
					podWithRevision("data-0", "rev-a"),
					podWithRevision("data-1", "rev-a"),
				},
			},
			want: []string{"data-0", "data-1"},
		},
		{
			name: "pods of a StatefulSet that is not expected anymore are not upgraded if they are members of the cluster",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
					sset.TestSset{Name: "data-new", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
				},
				expectedStatefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data-new", Namespace: TestEsNamespace, Replicas: 2}.Build(),
				},
				pods: []client.Object{
					podWithRevision("data-0", "rev-a"),
					podWithRevision("data-1", "rev-a"),
					podWithRevision("data-new-0", "rev-a"),
					podWithRevision("data-new-1", "rev-a"),
				},
				members: set.Make("data-0", "data-1"),
			},
			want: []string{"data-new-0", "data-new-1"},
		},
		{
			name: "pods of a StatefulSet that is not expected anymore are upgraded if they are not members of the cluster",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
					sset.TestSset{Name: "data-new", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
				},
				expectedStatefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data-new", Namespace: TestEsNamespace, Replicas: 2}.Build(),
				},
				pods: []client.Object{
					podWithRevision("data-0", "rev-a"),
					podWithRevision("data-1", "rev-a"),
					podWithRevision("data-new-0", "rev-a"),
					podWithRevision("data-new-1", "rev-a"),
				},
				// the Pod of data-1 never joined the cluster
				members: set.Make("data-0"),
			},
			want: []string{"data-1", "data-new-0", "data-new-1"},
		},
		{
			name: "pods whose ordinal is at or above the expected number of replicas are not upgraded if they are members of the cluster",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
				},
				expectedStatefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 1}.Build(),
				},
				pods: []client.Object{
					podWithRevision("data-0", "rev-a"),
					podWithRevision("data-1", "rev-a"),
				},
				members: set.Make("data-1"),
			},
			want: []string{"data-0"},
		},
		{
			name: "pods whose ordinal is at or above the expected number of replicas are upgraded if they are not members of the cluster",
			args: args{
				statefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 2, Status: outdated}.Build(),
				},
				expectedStatefulSets: es_sset.StatefulSetList{
					sset.TestSset{Name: "data", Namespace: TestEsNamespace, Replicas: 1}.Build(),
				},
				pods: []client.Object{
					podWithRevision("data-0", "rev-a"),
					podWithRevision("data-1", "rev-a"),
				},
			},
			want: []string{"data-0", "data-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.args.pods...)
			desiredLeaving := desiredLeavingNodeNames(context.Background(), tt.args.expectedStatefulSets, tt.args.statefulSets)
			got, err := podsToRollingUpgrade(client, tt.args.statefulSets, newLeavingNodes(desiredLeaving, clusterMembers{members: tt.args.members}, nil, esv1.DownscaleOperation{}))
			require.NoError(t, err)
			assert.ElementsMatch(t, names(got), tt.want, tt.name)
		})
	}
}

// Test_Driver_handleUpgrades_leavingPodsAreNotRestarted drives handleUpgrades with a nodeSet that has been removed from the
// spec while a rolling upgrade was still pending on it: its Pod must be left to the downscale if it is a member of the
// cluster, while the outdated Pod of a nodeSet that stays is still upgraded. The leaving Pod is the only candidate once
// filtered, so that the outcome does not depend on the order in which candidates are considered.
func Test_Driver_handleUpgrades_leavingPodsAreNotRestarted(t *testing.T) {
	esVersion := "8.1.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec:   esv1.ElasticsearchSpec{Version: esVersion},
		Status: esv1.ElasticsearchStatus{Version: esVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 1}
	upToDate := appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 1, Replicas: 1}
	statefulSet := func(name string, status appsv1.StatefulSetStatus) appsv1.StatefulSet {
		return sset.TestSset{Name: name, Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 1, Data: true, Status: status}.Build()
	}
	pod := func(ssetName, revision string, ready bool) corev1.Pod {
		return sset.TestPod{Name: ssetName + "-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: ssetName, Version: esVersion, Revision: revision, Data: true, Ready: ready}.Build()
	}
	tests := []struct {
		name              string
		leaving           appsv1.StatefulSetStatus // status of "data", which is absent from the expected StatefulSets
		staying           appsv1.StatefulSetStatus // status of "data-new", which is expected
		shutdowns         map[string]esclient.NodeShutdown
		leavingNotMember  bool           // the leaving Pod never joined the cluster, it is not ready
		wantShutdowns     []shutdownCall // shutdowns requested, by node ID and type
		wantRemainingPods []string       // Pods that have not been deleted for an upgrade
	}{
		{
			// the removal is not requested yet, for example because the downscale budget does not allow it
			name:              "only the leaving Pod is outdated and it is a member of the cluster: nothing is restarted",
			leaving:           outdated,
			staying:           upToDate,
			wantShutdowns:     nil,
			wantRemainingPods: []string{"data-0", "data-new-0"},
		},
		{
			name:    "only the leaving Pod is outdated and it is draining: nothing is restarted",
			leaving: outdated,
			staying: upToDate,
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-0": {NodeID: "id-data-0", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdowns:     nil,
			wantRemainingPods: []string{"data-0", "data-new-0"},
		},
		{
			// it has no shutdown to request, and it may need the upgrade to recover
			name:              "only the leaving Pod is outdated and it is not a member of the cluster: it is recreated",
			leaving:           outdated,
			staying:           upToDate,
			leavingNotMember:  true,
			wantShutdowns:     nil,
			wantRemainingPods: []string{"data-new-0"},
		},
		{
			name:              "only the staying Pod is outdated: it is restarted",
			leaving:           upToDate,
			staying:           outdated,
			wantShutdowns:     []shutdownCall{{NodeID: "id-data-new-0", Type: esclient.Restart}},
			wantRemainingPods: []string{"data-0"},
		},
		{
			name:    "both Pods are outdated and the leaving Pod is draining: only the staying Pod is restarted",
			leaving: outdated,
			staying: outdated,
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-0": {NodeID: "id-data-0", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantShutdowns:     []shutdownCall{{NodeID: "id-data-new-0", Type: esclient.Restart}},
			wantRemainingPods: []string{"data-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leavingSset, stayingSset := statefulSet("data", tt.leaving), statefulSet("data-new", tt.staying)
			leavingPod, stayingPod := pod("data", tt.leaving.CurrentRevision, !tt.leavingNotMember), pod("data-new", tt.staying.CurrentRevision, true)
			k8sClient := k8s.NewFakeClient(&es, &leavingSset, &stayingSset, &leavingPod, &stayingPod)
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{"id-data-0": {Name: "data-0"}, "id-data-new-0": {Name: "data-new-0"}}}
			if tt.leavingNotMember {
				delete(nodes.Nodes, "id-data-0")
			}
			esClient := &fakeESClient{
				version: version.MustParse(esVersion),
				nodes:   nodes,
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
				// the fake only creates a record for a node that has none
				Shutdowns: maps.Clone(tt.shutdowns),
			}
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			// the spec only has data-new left: data is being removed
			expectedResources := nodespec.ResourcesList{{StatefulSet: stayingSset}}

			desiredLeaving := desiredLeavingNodeNames(context.Background(), expectedResources.StatefulSets(), es_sset.StatefulSetList{leavingSset, stayingSset})
			esState := NewMemoizingESState(context.Background(), esClient)
			results := d.handleUpgrades(context.Background(), esClient, esState, expectedResources, newLeavingNodes(desiredLeaving, esState, nil, esv1.DownscaleOperation{}))

			_, err := results.Aggregate()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantShutdowns, esClient.PutShutdownCalls)
			// the removal of the leaving Pod is left untouched
			for nodeID, want := range tt.shutdowns {
				assert.Equal(t, want, esClient.Shutdowns[nodeID])
			}
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &pods))
			assert.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_leavingPodThatIsNotInCluster drives handleUpgrades with healthy data-0 and data-1, and data-2
// leaving while its outdated spec is repaired: the nodeSet is reduced from 3 to 2 with maxUnavailable 0. Neither the
// downscale budget nor the forced upgrade, which needs all the Pods to be unready, can remove or recreate data-2: the
// rolling upgrade must recreate it, for it to join the cluster, unless it is a member of the cluster.
func Test_Driver_handleUpgrades_leavingPodThatIsNotInCluster(t *testing.T) {
	esVersion := "8.1.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec: esv1.ElasticsearchSpec{
			Version:        esVersion,
			UpdateStrategy: esv1.UpdateStrategy{ChangeBudget: esv1.ChangeBudget{MaxUnavailable: new(int32(0))}},
		},
		Status: esv1.ElasticsearchStatus{Version: esVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 3}
	actualSset := sset.TestSset{Name: "data", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 3, Data: true, Status: outdated}.Build()
	expectedSset := sset.TestSset{Name: "data", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 2, Data: true}.Build()
	tests := []struct {
		name string
		// leavingPod customizes data-2, which is ready by default
		leavingPod func(pod *sset.TestPod)
		// esRunning makes the Elasticsearch container of data-2 run
		esRunning bool
		// joined makes data-2 a member of the cluster
		joined    bool
		shutdowns map[string]esclient.NodeShutdown
		// wantRecreated is true if data-2 is deleted for the upgrade
		wantRecreated bool
	}{
		{
			name: "Pending, Elasticsearch does not run in it: it is recreated",
			leavingPod: func(pod *sset.TestPod) {
				pod.Phase, pod.Ready = corev1.PodPending, false
			},
			wantRecreated: true,
		},
		{
			name: "crash looping, it never joined the cluster: it is recreated",
			leavingPod: func(pod *sset.TestPod) {
				pod.Ready, pod.RestartCount = false, 3
			},
			wantRecreated: true,
		},
		{
			name: "Pending, Elasticsearch runs in it and it is draining: it is not recreated",
			leavingPod: func(pod *sset.TestPod) {
				pod.Phase, pod.Ready = corev1.PodPending, false
			},
			esRunning: true,
			joined:    true,
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
			},
			wantRecreated: false,
		},
		{
			name:          "not ready but a member of the cluster, it is not draining: it is not recreated",
			leavingPod:    func(pod *sset.TestPod) { pod.Ready = false },
			joined:        true,
			wantRecreated: false,
		},
		{
			// the shutdown closed its readiness port
			name:       "not ready because it is draining: it is not recreated",
			leavingPod: func(pod *sset.TestPod) { pod.Ready = false },
			joined:     true,
			shutdowns: map[string]esclient.NodeShutdown{
				"id-data-2": {NodeID: "id-data-2", Type: string(esclient.Remove), Status: esclient.ShutdownStalled},
			},
			wantRecreated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPod := func(ordinal int32) sset.TestPod {
				return sset.TestPod{
					Name: sset.PodName("data", ordinal), Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "data",
					Version: esVersion, Revision: "rev-a", Data: true, Ready: true,
				}
			}
			leavingTestPod := testPod(2)
			tt.leavingPod(&leavingTestPod)
			pods := []corev1.Pod{testPod(0).Build(), testPod(1).Build(), leavingTestPod.Build()}
			if tt.esRunning {
				pods[2].Status.ContainerStatuses[0].State.Running = &corev1.ContainerStateRunning{}
			}
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{"id-data-0": {Name: "data-0"}, "id-data-1": {Name: "data-1"}}}
			if tt.joined {
				nodes.Nodes["id-data-2"] = esclient.Node{Name: "data-2"}
			}
			esClient := &fakeESClient{
				version:   version.MustParse(esVersion),
				nodes:     nodes,
				health:    esclient.Health{Status: esv1.ElasticsearchGreenHealth},
				Shutdowns: maps.Clone(tt.shutdowns),
			}
			// the forced upgrade does not apply, the rolling upgrade is the only way to recreate data-2
			require.False(t, shouldForceUpgrade(pods))
			k8sClient := k8s.NewFakeClient(&es, &actualSset, &pods[0], &pods[1], &pods[2])
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			expectedResources := nodespec.ResourcesList{{StatefulSet: expectedSset}}

			desiredLeaving := desiredLeavingNodeNames(context.Background(), expectedResources.StatefulSets(), es_sset.StatefulSetList{actualSset})
			require.ElementsMatch(t, []string{"data-2"}, desiredLeaving.AsSortedSlice())
			esState := NewMemoizingESState(context.Background(), esClient)
			results := d.handleUpgrades(context.Background(), esClient, esState, expectedResources, newLeavingNodes(desiredLeaving, esState, nil, esv1.DownscaleOperation{}))

			_, err := results.Aggregate()
			require.NoError(t, err)
			// the Pods that are ready wait for the budget, and no restart is requested for data-2: it is not ready
			assert.Empty(t, esClient.PutShutdownCalls)
			// the removal of a draining Pod is left untouched
			assert.Equal(t, tt.shutdowns == nil, len(esClient.Shutdowns) == 0)
			for nodeID, want := range tt.shutdowns {
				assert.Equal(t, want, esClient.Shutdowns[nodeID])
			}
			wantRemainingPods := []string{"data-0", "data-1", "data-2"}
			if tt.wantRecreated {
				wantRemainingPods = []string{"data-0", "data-1"}
			}
			var actualPods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &actualPods))
			assert.ElementsMatch(t, wantRemainingPods, names(actualPods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_leavingMasterThatIsNotInCluster drives handleUpgrades with healthy master-0 and master-1, and
// master-2 leaving while its outdated spec is repaired: the nodeSet is reduced from 3 to 2. master-2 is recreated if it is
// not a member of the cluster, without excluding it from the voting, as it is not part of the voting configuration, which
// Elasticsearch would reject for a node it does not know.
func Test_Driver_handleUpgrades_leavingMasterThatIsNotInCluster(t *testing.T) {
	esVersion := "8.1.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec:   esv1.ElasticsearchSpec{Version: esVersion},
		Status: esv1.ElasticsearchStatus{Version: esVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 3}
	actualSset := sset.TestSset{Name: "master", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 3, Master: true, Data: true, Status: outdated}.Build()
	expectedSset := sset.TestSset{Name: "master", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 2, Master: true, Data: true}.Build()
	tests := []struct {
		name string
		// joined makes master-2 a member of the cluster, it is not ready otherwise
		joined bool
		// wantRecreated is true if master-2 is deleted for the upgrade
		wantRecreated bool
	}{
		{
			name:          "master-2 is not a member of the cluster: it is recreated, without excluding it from the voting",
			wantRecreated: true,
		},
		{
			name:          "master-2 is a member of the cluster: it is left to the downscale",
			joined:        true,
			wantRecreated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testPod := func(ordinal int32) sset.TestPod {
				return sset.TestPod{
					Name: sset.PodName("master", ordinal), Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "master",
					Version: esVersion, Revision: "rev-a", Master: true, Data: true, Ready: true,
				}
			}
			leavingTestPod := testPod(2)
			leavingTestPod.Ready = tt.joined
			if !tt.joined {
				leavingTestPod.Phase = corev1.PodPending
			}
			pods := []corev1.Pod{testPod(0).Build(), testPod(1).Build(), leavingTestPod.Build()}
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{"id-master-0": {Name: "master-0"}, "id-master-1": {Name: "master-1"}}}
			if tt.joined {
				nodes.Nodes["id-master-2"] = esclient.Node{Name: "master-2"}
			}
			esClient := &fakeESClient{
				version: version.MustParse(esVersion),
				nodes:   nodes,
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			}
			k8sClient := k8s.NewFakeClient(&es, &actualSset, &pods[0], &pods[1], &pods[2])
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			expectedResources := nodespec.ResourcesList{{StatefulSet: expectedSset}}

			desiredLeaving := desiredLeavingNodeNames(context.Background(), expectedResources.StatefulSets(), es_sset.StatefulSetList{actualSset})
			require.ElementsMatch(t, []string{"master-2"}, desiredLeaving.AsSortedSlice())
			esState := NewMemoizingESState(context.Background(), esClient)
			results := d.handleUpgrades(context.Background(), esClient, esState, expectedResources, newLeavingNodes(desiredLeaving, esState, nil, esv1.DownscaleOperation{}))

			_, err := results.Aggregate()
			require.NoError(t, err)
			assert.False(t, esClient.AddVotingConfigExclusionsCalled)
			wantRemainingPods := []string{"master-0", "master-1", "master-2"}
			if tt.wantRecreated {
				wantRemainingPods = []string{"master-0", "master-1"}
			}
			var actualPods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &actualPods))
			assert.ElementsMatch(t, wantRemainingPods, names(actualPods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_restartShutdownCompletesAfterRequest covers a restart shutdown that is not complete right
// after it is requested, which holds the Pod: Elasticsearch only reports it as complete a short while later. The next
// pass must delete the Pod once the shutdown is complete, without deleting the completed shutdown and requesting it
// again, which would hold the Pod again. If a predicate holds the Pod on the next pass, the shutdown is deleted instead:
// it would keep the readiness port of the Pod closed while it waits.
func Test_Driver_handleUpgrades_restartShutdownCompletesAfterRequest(t *testing.T) {
	esVersion := "8.1.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec:   esv1.ElasticsearchSpec{Version: esVersion},
		Status: esv1.ElasticsearchStatus{Version: esVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 1}
	statefulSet := sset.TestSset{Name: "data", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 1, Data: true, Status: outdated}.Build()
	testPod := sset.TestPod{
		Name: "data-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "data",
		Version: esVersion, Revision: "rev-a", Data: true, Ready: true,
	}
	pod := testPod.Build()
	// a primary on data-0 whose replica is not assigned, which require_started_replica holds while the cluster is not green
	unassignedReplica := esclient.Shards{
		{Index: "index", Shard: "0", State: esclient.STARTED, NodeName: "data-0", Type: esclient.Primary},
		{Index: "index", Shard: "0", State: esclient.UNASSIGNED, Type: esclient.Replica},
	}
	tests := []struct {
		name string
		// statusOnNextPass is the status of the shutdown when the next pass retrieves it
		statusOnNextPass esclient.ShutdownStatus
		// notReadyOnNextPass makes the Pod not ready on the next pass, as the shutdown closed its readiness port
		notReadyOnNextPass bool
		// healthOnNextPass is the cluster health on the next pass, green if empty
		healthOnNextPass esv1.ElasticsearchHealth
		// shardsOnNextPass are the shards on the next pass
		shardsOnNextPass  esclient.Shards
		wantRemainingPods []string // Pods that have not been deleted for an upgrade after the next pass
		// wantShutdownKept is true if the restart shutdown still exists after the next pass
		wantShutdownKept bool
	}{
		{
			name:              "the shutdown is complete on the next pass: the Pod is deleted",
			statusOnNextPass:  esclient.ShutdownComplete,
			wantRemainingPods: nil,
			wantShutdownKept:  true,
		},
		{
			name:              "the shutdown is still in progress on the next pass: the Pod is held",
			statusOnNextPass:  esclient.ShutdownInProgress,
			wantRemainingPods: []string{"data-0"},
			wantShutdownKept:  true,
		},
		{
			name:               "the shutdown is complete and closed the readiness port on the next pass: the Pod is deleted",
			statusOnNextPass:   esclient.ShutdownComplete,
			notReadyOnNextPass: true,
			wantRemainingPods:  nil,
			wantShutdownKept:   true,
		},
		{
			name:               "a predicate holds the Pod on the next pass: the completed shutdown is deleted",
			statusOnNextPass:   esclient.ShutdownComplete,
			notReadyOnNextPass: true,
			healthOnNextPass:   esv1.ElasticsearchYellowHealth,
			shardsOnNextPass:   unassignedReplica,
			wantRemainingPods:  []string{"data-0"},
			wantShutdownKept:   false,
		},
		{
			name:               "a predicate holds the Pod on the next pass, the shutdown is in progress: it is deleted",
			statusOnNextPass:   esclient.ShutdownInProgress,
			notReadyOnNextPass: true,
			healthOnNextPass:   esv1.ElasticsearchYellowHealth,
			shardsOnNextPass:   unassignedReplica,
			wantRemainingPods:  []string{"data-0"},
			wantShutdownKept:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := k8s.NewFakeClient(&es, &statefulSet, &pod)
			esClient := &fakeESClient{
				version: version.MustParse(esVersion),
				nodes:   esclient.Nodes{Nodes: map[string]esclient.Node{"id-data-0": {Name: "data-0"}}},
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
				// the shutdown is reported in progress right after it is requested
				RegisterPutShutdowns: true,
			}
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			expectedResources := nodespec.ResourcesList{{StatefulSet: statefulSet}}
			handleUpgrades := func() {
				esState := NewMemoizingESState(ctx, esClient)
				results := d.handleUpgrades(ctx, esClient, esState, expectedResources, newLeavingNodes(set.Make(), esState, nil, esv1.DownscaleOperation{}))
				_, err := results.Aggregate()
				require.NoError(t, err)
			}

			// the Pod is held until the shutdown is complete
			handleUpgrades()
			require.Equal(t, []shutdownCall{{NodeID: "id-data-0", Type: esclient.Restart}}, esClient.PutShutdownCalls)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(ctx, &pods))
			require.ElementsMatch(t, []string{"data-0"}, names(pods.Items))

			restartShutdown := esClient.Shutdowns["id-data-0"]
			restartShutdown.Status = tt.statusOnNextPass
			esClient.Shutdowns["id-data-0"] = restartShutdown
			esClient.health = esclient.Health{Status: cmp.Or(tt.healthOnNextPass, esv1.ElasticsearchGreenHealth)}
			esClient.shards = tt.shardsOnNextPass
			if tt.notReadyOnNextPass {
				var actual corev1.Pod
				require.NoError(t, k8sClient.Get(ctx, k8s.ExtractNamespacedName(&pod), &actual))
				notReady := testPod
				notReady.Ready = false
				actual.Status = notReady.Build().Status
				require.NoError(t, k8sClient.Status().Update(ctx, &actual))
			}
			handleUpgrades()
			// the shutdown is not requested again
			assert.Len(t, esClient.PutShutdownCalls, 1)
			_, shutdownKept := esClient.Shutdowns["id-data-0"]
			assert.Equal(t, tt.wantShutdownKept, shutdownKept)
			require.NoError(t, k8sClient.List(ctx, &pods))
			assert.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_heldRestartShutdowns covers the restart shutdowns of the Pods that are not deleted on the
// pass after the one that requested them. The shutdown of a Pod that fails a predicate is deleted, as it would keep the
// Pod unavailable, which can hold the restarts the predicate waits for. The shutdown of a Pod that is only held because
// allowedDeletions is reached is kept: the Pod still waits for its deletion.
func Test_Driver_handleUpgrades_heldRestartShutdowns(t *testing.T) {
	esVersion := "8.1.0"
	outdated := func(replicas int32) appsv1.StatefulSetStatus {
		return appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: replicas}
	}
	upToDate := func(replicas int32) appsv1.StatefulSetStatus {
		return appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-a", UpdatedReplicas: replicas, Replicas: replicas}
	}
	tests := []struct {
		name           string
		maxUnavailable *int32
		// statefulSets are the StatefulSets on the first pass, and statefulSetsOnNextPass their changes on the next one
		statefulSets           []sset.TestSset
		statefulSetsOnNextPass []sset.TestSset
		// statusOnNextPass is the status of the shutdowns when the next pass retrieves them
		statusOnNextPass esclient.ShutdownStatus
		// wantPutShutdowns are the restart shutdowns requested by the first pass
		wantPutShutdowns []string
		// wantDeleted are the Pods deleted by the next pass
		wantDeleted []string
		// wantShutdownsOnNextPass are the nodes that have a restart shutdown after the next pass
		wantShutdownsOnNextPass []string
		// wantPutShutdownsOnLastPass are the restart shutdowns requested once the Pods without a shutdown are ready again
		wantPutShutdownsOnLastPass []string
	}{
		{
			// the master is not ready because of its shutdown, which uses up maxUnavailable and holds the restart of the
			// data Pod, which the master waits for to be upgraded last
			name: "a data Pod change after the master shutdown, which is complete: the master shutdown is deleted",
			statefulSets: []sset.TestSset{
				{Name: "master", Replicas: 1, Master: true, Status: outdated(1)},
				{Name: "data", Replicas: 1, Data: true, Status: upToDate(1)},
			},
			statefulSetsOnNextPass: []sset.TestSset{
				{Name: "master", Replicas: 1, Master: true, Status: outdated(1)},
				{Name: "data", Replicas: 1, Data: true, Status: outdated(1)},
			},
			statusOnNextPass:           esclient.ShutdownComplete,
			wantPutShutdowns:           []string{"id-master-0"},
			wantDeleted:                nil,
			wantShutdownsOnNextPass:    nil,
			wantPutShutdownsOnLastPass: []string{"id-master-0", "id-data-0"},
		},
		{
			name: "a data Pod change after the master shutdown, which is in progress: the master shutdown is deleted",
			statefulSets: []sset.TestSset{
				{Name: "master", Replicas: 1, Master: true, Status: outdated(1)},
				{Name: "data", Replicas: 1, Data: true, Status: upToDate(1)},
			},
			statefulSetsOnNextPass: []sset.TestSset{
				{Name: "master", Replicas: 1, Master: true, Status: outdated(1)},
				{Name: "data", Replicas: 1, Data: true, Status: outdated(1)},
			},
			statusOnNextPass:           esclient.ShutdownInProgress,
			wantPutShutdowns:           []string{"id-master-0"},
			wantDeleted:                nil,
			wantShutdownsOnNextPass:    nil,
			wantPutShutdownsOnLastPass: []string{"id-master-0", "id-data-0"},
		},
		{
			// data-2 and data-1 are not ready because of their shutdowns, which uses up maxUnavailable: data-2 is the
			// only one deleted, and data-1 waits for the next pass. data-0 is kept for the tier availability.
			name:           "two Pods with a restart shutdown and maxUnavailable reached: the shutdown of the second one is kept",
			maxUnavailable: new(int32(2)),
			statefulSets: []sset.TestSset{
				{Name: "data", Replicas: 3, Data: true, Status: outdated(3)},
			},
			statusOnNextPass:        esclient.ShutdownComplete,
			wantPutShutdowns:        []string{"id-data-2", "id-data-1"},
			wantDeleted:             []string{"data-2"},
			wantShutdownsOnNextPass: []string{"id-data-2", "id-data-1"},
			// data-2 is not recreated yet, nothing more is requested
			wantPutShutdownsOnLastPass: []string{"id-data-2", "id-data-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			es := esv1.Elasticsearch{
				Name: TestEsName, Namespace: TestEsNamespace,
				Spec: esv1.ElasticsearchSpec{
					Version:        esVersion,
					UpdateStrategy: esv1.UpdateStrategy{ChangeBudget: esv1.ChangeBudget{MaxUnavailable: tt.maxUnavailable}},
				},
				Status: esv1.ElasticsearchStatus{Version: esVersion},
			}
			buildStatefulSet := func(testSset sset.TestSset) appsv1.StatefulSet {
				testSset.Namespace, testSset.ClusterName, testSset.Version = TestEsNamespace, TestEsName, esVersion
				return testSset.Build()
			}
			testPod := func(testSset sset.TestSset, ordinal int32, ready bool) sset.TestPod {
				return sset.TestPod{
					Name: sset.PodName(testSset.Name, ordinal), Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: testSset.Name,
					Version: esVersion, Revision: "rev-a", Master: testSset.Master, Data: testSset.Data, Ready: ready,
				}
			}
			objects := []client.Object{&es}
			nodes := esclient.Nodes{Nodes: map[string]esclient.Node{}}
			var expectedResources nodespec.ResourcesList
			for _, testSset := range tt.statefulSets {
				statefulSet := buildStatefulSet(testSset)
				objects = append(objects, &statefulSet)
				expectedResources = append(expectedResources, nodespec.Resources{StatefulSet: statefulSet})
				for ordinal := range testSset.Replicas {
					objects = append(objects, testPod(testSset, ordinal, true).BuildPtr())
					nodes.Nodes["id-"+sset.PodName(testSset.Name, ordinal)] = esclient.Node{Name: sset.PodName(testSset.Name, ordinal)}
				}
			}
			k8sClient := k8s.NewFakeClient(objects...)
			esClient := &fakeESClient{
				version: version.MustParse(esVersion),
				nodes:   nodes,
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
				// the shutdowns are reported in progress right after they are requested
				RegisterPutShutdowns: true,
			}
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			handleUpgrades := func() {
				esState := NewMemoizingESState(ctx, esClient)
				results := d.handleUpgrades(ctx, esClient, esState, expectedResources, newLeavingNodes(set.Make(), esState, nil, esv1.DownscaleOperation{}))
				_, err := results.Aggregate()
				require.NoError(t, err)
			}
			putShutdownNodes := func() []string {
				nodeIDs := make([]string, 0, len(esClient.PutShutdownCalls))
				for _, call := range esClient.PutShutdownCalls {
					require.Equal(t, esclient.Restart, call.Type)
					nodeIDs = append(nodeIDs, call.NodeID)
				}
				return nodeIDs
			}
			// syncReadiness makes the Pods that have a shutdown not ready, as it closes their readiness port, and the others
			// ready
			syncReadiness := func() {
				for _, testSset := range tt.statefulSets {
					for ordinal := range testSset.Replicas {
						podName := sset.PodName(testSset.Name, ordinal)
						var actual corev1.Pod
						err := k8sClient.Get(ctx, types.NamespacedName{Namespace: TestEsNamespace, Name: podName}, &actual)
						if apierrors.IsNotFound(err) {
							// deleted for its upgrade
							continue
						}
						require.NoError(t, err)
						_, hasShutdown := esClient.Shutdowns["id-"+podName]
						actual.Status = testPod(testSset, ordinal, !hasShutdown).Build().Status
						require.NoError(t, k8sClient.Status().Update(ctx, &actual))
					}
				}
			}
			podNames := func() []string {
				var pods corev1.PodList
				require.NoError(t, k8sClient.List(ctx, &pods))
				return names(pods.Items)
			}

			// the shutdowns are in progress: no Pod is deleted
			handleUpgrades()
			require.ElementsMatch(t, tt.wantPutShutdowns, putShutdownNodes())
			allPods := podNames()

			for _, testSset := range tt.statefulSetsOnNextPass {
				statefulSet := buildStatefulSet(testSset)
				var actual appsv1.StatefulSet
				require.NoError(t, k8sClient.Get(ctx, k8s.ExtractNamespacedName(&statefulSet), &actual))
				actual.Status = statefulSet.Status
				require.NoError(t, k8sClient.Status().Update(ctx, &actual))
			}
			for nodeID, shutdown := range esClient.Shutdowns {
				shutdown.Status = tt.statusOnNextPass
				esClient.Shutdowns[nodeID] = shutdown
			}
			syncReadiness()
			handleUpgrades()
			// the shutdowns are not requested again
			assert.ElementsMatch(t, tt.wantPutShutdowns, putShutdownNodes())
			deleted, _ := stringsutil.Difference(allPods, podNames())
			assert.ElementsMatch(t, tt.wantDeleted, deleted)
			assert.ElementsMatch(t, tt.wantShutdownsOnNextPass, slices.Collect(maps.Keys(esClient.Shutdowns)))

			// the Pods whose shutdown was deleted are ready again: the restarts they held can be requested
			syncReadiness()
			handleUpgrades()
			assert.ElementsMatch(t, tt.wantPutShutdownsOnLastPass, putShutdownNodes())
		})
	}
}

// Test_Driver_handleUpgrades_fullRestartShutdownsCompleteAfterRequest covers the full restart of a non-HA cluster, which
// only deletes the Pods once all their shutdowns are complete. The shutdowns are not complete right after they are
// requested, which holds all the Pods: Elasticsearch only reports them as complete a short while later. The next pass
// must delete all the Pods once all the shutdowns are complete, without deleting the completed shutdowns and requesting
// them again, which would hold the Pods again.
func Test_Driver_handleUpgrades_fullRestartShutdownsCompleteAfterRequest(t *testing.T) {
	fromVersion, toVersion := "8.1.0", "8.2.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec:   esv1.ElasticsearchSpec{Version: toVersion},
		Status: esv1.ElasticsearchStatus{Version: fromVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 1}
	// a single master makes the cluster non-HA
	masterSset := sset.TestSset{Name: "master", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: toVersion, Replicas: 1, Master: true, Data: true, Status: outdated}.Build()
	dataSset := sset.TestSset{Name: "data", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: toVersion, Replicas: 1, Data: true, Status: outdated}.Build()
	masterPod := sset.TestPod{Name: "master-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "master", Version: fromVersion, Revision: "rev-a", Master: true, Data: true, Ready: true}.Build()
	dataPod := sset.TestPod{Name: "data-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "data", Version: fromVersion, Revision: "rev-a", Data: true, Ready: true}.Build()
	tests := []struct {
		name string
		// statusesOnNextPass are the statuses of the shutdowns, by node ID, when the next pass retrieves them
		statusesOnNextPass map[string]esclient.ShutdownStatus
		wantRemainingPods  []string // Pods that have not been deleted for an upgrade after the next pass
	}{
		{
			name: "all the shutdowns are complete on the next pass: all the Pods are deleted",
			statusesOnNextPass: map[string]esclient.ShutdownStatus{
				"id-master-0": esclient.ShutdownComplete,
				"id-data-0":   esclient.ShutdownComplete,
			},
			wantRemainingPods: nil,
		},
		{
			name: "a shutdown is still in progress on the next pass: no Pod is deleted",
			statusesOnNextPass: map[string]esclient.ShutdownStatus{
				"id-master-0": esclient.ShutdownComplete,
				"id-data-0":   esclient.ShutdownInProgress,
			},
			wantRemainingPods: []string{"master-0", "data-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k8sClient := k8s.NewFakeClient(&es, &masterSset, &dataSset, &masterPod, &dataPod)
			esClient := &fakeESClient{
				version: version.MustParse(fromVersion),
				nodes:   esclient.Nodes{Nodes: map[string]esclient.Node{"id-master-0": {Name: "master-0"}, "id-data-0": {Name: "data-0"}}},
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
				// the shutdowns are reported in progress right after they are requested
				RegisterPutShutdowns: true,
			}
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			expectedResources := nodespec.ResourcesList{{StatefulSet: masterSset}, {StatefulSet: dataSset}}
			// make sure the fixture really selects the full-restart branch
			currentPods, err := es_sset.StatefulSetList{masterSset, dataSset}.GetActualPods(k8sClient)
			require.NoError(t, err)
			isUpgrade, err := isVersionUpgrade(es)
			require.NoError(t, err)
			require.True(t, isUpgrade)
			require.True(t, isNonHACluster(currentPods, expectedResources.MasterNodesNames()))
			handleUpgrades := func() {
				esState := NewMemoizingESState(ctx, esClient)
				results := d.handleUpgrades(ctx, esClient, esState, expectedResources, newLeavingNodes(set.Make(), esState, nil, esv1.DownscaleOperation{}))
				_, err := results.Aggregate()
				require.NoError(t, err)
			}

			// no Pod is deleted until all the shutdowns are complete
			handleUpgrades()
			wantPutShutdowns := []shutdownCall{{NodeID: "id-master-0", Type: esclient.Restart}, {NodeID: "id-data-0", Type: esclient.Restart}}
			require.ElementsMatch(t, wantPutShutdowns, esClient.PutShutdownCalls)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(ctx, &pods))
			require.ElementsMatch(t, []string{"master-0", "data-0"}, names(pods.Items))

			for nodeID, status := range tt.statusesOnNextPass {
				restartShutdown := esClient.Shutdowns[nodeID]
				restartShutdown.Status = status
				esClient.Shutdowns[nodeID] = restartShutdown
			}
			handleUpgrades()
			// the shutdowns are not requested again
			assert.ElementsMatch(t, wantPutShutdowns, esClient.PutShutdownCalls)
			for nodeID := range tt.statusesOnNextPass {
				assert.Contains(t, esClient.Shutdowns, nodeID)
			}
			require.NoError(t, k8sClient.List(ctx, &pods))
			assert.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_leavingPodsSurviveFullClusterRestart covers the other deletion branch of handleUpgrades: a
// version upgrade of a non-HA cluster restarts all outdated Pods at once, without predicates. A Pod of a nodeSet that
// is being removed must not be part of that restart either, if it is a member of the cluster.
func Test_Driver_handleUpgrades_leavingPodsSurviveFullClusterRestart(t *testing.T) {
	fromVersion, toVersion := "8.1.0", "8.2.0"
	es := esv1.Elasticsearch{
		Name: TestEsName, Namespace: TestEsNamespace,
		Spec:   esv1.ElasticsearchSpec{Version: toVersion},
		Status: esv1.ElasticsearchStatus{Version: fromVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 1}
	// a single master makes the cluster non-HA; "gone" is absent from the expected StatefulSets
	masterSset := sset.TestSset{Name: "master", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: toVersion, Replicas: 1, Master: true, Data: true, Status: outdated}.Build()
	leavingSset := sset.TestSset{Name: "gone", Namespace: TestEsNamespace, ClusterName: TestEsName, Version: toVersion, Replicas: 1, Data: true, Status: outdated}.Build()
	masterPod := sset.TestPod{Name: "master-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "master", Version: fromVersion, Revision: "rev-a", Master: true, Data: true, Ready: true}.Build()
	leavingPod := sset.TestPod{Name: "gone-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: "gone", Version: fromVersion, Revision: "rev-a", Data: true, Ready: true}.Build()
	k8sClient := k8s.NewFakeClient(&es, &masterSset, &leavingSset, &masterPod, &leavingPod)
	esClient := &fakeESClient{
		version: version.MustParse(fromVersion),
		nodes:   esclient.Nodes{Nodes: map[string]esclient.Node{"id-master-0": {Name: "master-0"}, "id-gone-0": {Name: "gone-0"}}},
		health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
		// the removal of the leaving Pod is in progress
		Shutdowns: map[string]esclient.NodeShutdown{
			"id-gone-0": {NodeID: "id-gone-0", Type: string(esclient.Remove), Status: esclient.ShutdownInProgress},
		},
	}
	d := &Driver{
		Client:         k8sClient,
		ES:             es,
		Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
		ReconcileState: reconcile.MustNewState(es),
	}
	expectedResources := nodespec.ResourcesList{{StatefulSet: masterSset}}
	// make sure the fixture really selects the full-restart branch
	currentPods, err := es_sset.StatefulSetList{masterSset, leavingSset}.GetActualPods(k8sClient)
	require.NoError(t, err)
	isUpgrade, err := isVersionUpgrade(es)
	require.NoError(t, err)
	require.True(t, isUpgrade)
	require.True(t, isNonHACluster(currentPods, expectedResources.MasterNodesNames()))

	desiredLeaving := desiredLeavingNodeNames(context.Background(), expectedResources.StatefulSets(), es_sset.StatefulSetList{masterSset, leavingSset})
	esState := NewMemoizingESState(context.Background(), esClient)
	results := d.handleUpgrades(context.Background(), esClient, esState, expectedResources, newLeavingNodes(desiredLeaving, esState, nil, esv1.DownscaleOperation{}))

	_, err = results.Aggregate()
	require.NoError(t, err)
	assert.ElementsMatch(t, []shutdownCall{{NodeID: "id-master-0", Type: esclient.Restart}}, esClient.PutShutdownCalls)
	var pods corev1.PodList
	require.NoError(t, k8sClient.List(context.Background(), &pods))
	assert.ElementsMatch(t, []string{"gone-0"}, names(pods.Items))
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

func Test_Driver_maybeCompleteNodeUpgrades(t *testing.T) {
	esVersion := "8.1.0"
	clusterName = "test-cluster"
	namespace := "ns"
	es = esv1.Elasticsearch{
		Name:      clusterName,
		Namespace: namespace,
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
	twoShutdownsFixture := map[string]esclient.NodeShutdown{
		"node-id-0": {
			NodeID: "node-id-0",
			Type:   "RESTART",
			Status: "COMPLETE",
		},
		"node-id-1": {
			NodeID: "node-id-1",
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
		// pendingRestarts are the Pods that still wait for their restart
		pendingRestarts []string
		assertions      func(*reconciler.Results, *fakeESClient)
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
			// the shutdown of es-0 may have turned complete after it was requested: it is deleted once es-0 restarted
			name: "all nodes in cluster, completed shutdown of a Pod that waits for its restart is kept",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
				"node-id-1": {Name: "es-1"},
			},
			shutdowns:       twoShutdownsFixture,
			pendingRestarts: []string{"es-0"},
			runtimeObjects:  append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.True(t, esClient.DeleteShutdownCalled)
				// the completed shutdown of the other Pod is deleted
				require.Contains(t, esClient.Shutdowns, "node-id-0")
				require.NotContains(t, esClient.Shutdowns, "node-id-1")
				reconciled, _ := results.IsReconciled()
				require.True(t, reconciled)
			},
		},
		{
			name: "not all nodes in cluster, completed shutdown of a Pod that waits for its restart is kept",
			es:   es,
			nodesInCluster: map[string]esclient.Node{
				"node-id-0": {Name: "es-0"},
			},
			shutdowns:       shutdownFixture,
			pendingRestarts: []string{"es-0"},
			runtimeObjects:  append(testSset.Pods(), testSset.BuildPtr()),
			assertions: func(results *reconciler.Results, esClient *fakeESClient) {
				require.False(t, esClient.DeleteShutdownCalled)
				require.Contains(t, esClient.Shutdowns, "node-id-0")
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
				Shutdowns:                maps.Clone(tt.shutdowns),
				clusterRoutingAllocation: tt.routingAllocation,
			}
			esState := NewMemoizingESState(context.Background(), esClient)

			reconcileState, err := reconcile.NewState(tt.es)
			require.NoError(t, err)

			d := &Driver{

				Client:         client,
				ES:             tt.es,
				Expectations:   expectations.NewExpectations(client, &appsv1.StatefulSet{}),
				ReconcileState: reconcileState,
			}
			if tt.expectations != nil {
				tt.expectations(d.Expectations)
			}

			nodeNameToID, err := esState.NodeNameToID()
			require.NoError(t, err)

			n := shutdown.NewNodeShutdown(esClient, nodeNameToID, esclient.Restart, "", nil, crlog.Log)
			var pendingRestarts []corev1.Pod
			for _, name := range tt.pendingRestarts {
				pendingRestarts = append(pendingRestarts, corev1.Pod{Name: name})
			}
			results := d.maybeCompleteNodeUpgrades(context.Background(), esClient, esState, n, pendingRestarts)
			tt.assertions(results, esClient)
		})
	}
}

func Test_shutdownReasonAndAllocationDelay(t *testing.T) {
	tests := []struct {
		name                         string
		annotations                  map[string]string
		resourceVersion              string
		isAnnotationTriggeredRestart bool
		wantReason                   string
		wantDelay                    *time.Duration
	}{
		{
			name:                         "no annotations, uses resource version",
			annotations:                  nil,
			resourceVersion:              "12345",
			isAnnotationTriggeredRestart: false,
			wantReason:                   "12345",
			wantDelay:                    nil,
		},
		{
			name:                         "restart-trigger set and used as reason",
			annotations:                  map[string]string{esv1.RestartTriggerAnnotation: "2026-01-14T12:00:00Z"},
			resourceVersion:              "12345",
			isAnnotationTriggeredRestart: true,
			wantReason:                   "2026-01-14T12:00:00Z",
			wantDelay:                    nil,
		},
		{
			name:                         "restart-trigger set but not used as reason",
			annotations:                  map[string]string{esv1.RestartTriggerAnnotation: "2026-01-14T12:00:00Z"},
			resourceVersion:              "12345",
			isAnnotationTriggeredRestart: false,
			wantReason:                   "12345",
			wantDelay:                    nil,
		},
		{
			name:                         "restart-trigger empty, falls back to resource version",
			annotations:                  map[string]string{esv1.RestartTriggerAnnotation: ""},
			resourceVersion:              "99",
			isAnnotationTriggeredRestart: true,
			wantReason:                   "99",
			wantDelay:                    nil,
		},
		{
			name:                         "allocation delay set",
			annotations:                  map[string]string{esv1.RestartAllocationDelayAnnotation: "10m"},
			resourceVersion:              "42",
			isAnnotationTriggeredRestart: false,
			wantReason:                   "42",
			wantDelay:                    new(10 * time.Minute),
		},
		{
			name:                         "allocation delay set to negative",
			annotations:                  map[string]string{esv1.RestartAllocationDelayAnnotation: "-10m"},
			resourceVersion:              "42",
			isAnnotationTriggeredRestart: false,
			wantReason:                   "42",
			wantDelay:                    nil,
		},
		{
			name:                         "allocation delay invalid, ignored",
			annotations:                  map[string]string{esv1.RestartAllocationDelayAnnotation: "not-a-duration"},
			resourceVersion:              "42",
			isAnnotationTriggeredRestart: false,
			wantReason:                   "42",
			wantDelay:                    nil,
		},
		{
			name: "both annotations set",
			annotations: map[string]string{
				esv1.RestartTriggerAnnotation:         "trigger-value",
				esv1.RestartAllocationDelayAnnotation: "5m",
			},
			resourceVersion:              "42",
			isAnnotationTriggeredRestart: true,
			wantReason:                   "trigger-value",
			wantDelay:                    new(5 * time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				ResourceVersion: tt.resourceVersion,
				Annotations:     tt.annotations,
			}
			reason, delay := shutdownReasonAndAllocationDelay(es, tt.isAnnotationTriggeredRestart, crlog.Log)
			assert.Equal(t, tt.wantReason, reason)
			if tt.wantDelay == nil {
				assert.Nil(t, delay)
			} else {
				require.NotNil(t, delay)
				assert.Equal(t, *tt.wantDelay, *delay)
			}
		})
	}
}

func Test_isAnnotationTriggeredRestart(t *testing.T) {
	// Helper to build a StatefulSet with config-hash annotation on the template.
	stsWithHash := func(name, configHash string) appsv1.StatefulSet {
		sts := sset.TestSset{Name: name, Namespace: "ns", Replicas: 1}.Build()
		if sts.Spec.Template.Annotations == nil {
			sts.Spec.Template.Annotations = make(map[string]string)
		}
		sts.Spec.Template.Annotations[nodespec.ConfigHashAnnotationName] = configHash
		return sts
	}
	// Helper to build ResourcesList from StatefulSets.
	resourcesList := func(sts ...appsv1.StatefulSet) nodespec.ResourcesList {
		out := make(nodespec.ResourcesList, len(sts))
		for i, s := range sts {
			out[i] = nodespec.Resources{StatefulSet: s}
		}
		return out
	}
	// Helper to build a pod with labels and annotations.
	pod := func(name, ssetName, configHash, restartTrigger string) corev1.Pod {
		p := sset.TestPod{Name: name, Namespace: "ns", StatefulSetName: ssetName}.Build()
		p.Labels[label.StatefulSetNameLabelName] = ssetName
		p.Annotations = make(map[string]string)
		if configHash != "" {
			p.Annotations[nodespec.ConfigHashAnnotationName] = configHash
		}
		if restartTrigger != "" {
			p.Annotations[esv1.RestartTriggerAnnotation] = restartTrigger
		}
		return p
	}

	tests := []struct {
		name          string
		es            esv1.Elasticsearch
		resourcesList nodespec.ResourcesList
		podsToUpgrade []corev1.Pod
		want          bool
	}{
		{
			name: "no restart annotation on ES",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Annotations: nil},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: []corev1.Pod{pod("masters-0", "masters", "hash1", "old")},
			want:          false,
		},
		{
			name: "empty restart annotation on ES",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: ""},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: []corev1.Pod{pod("masters-0", "masters", "hash1", "")},
			want:          false,
		},
		{
			name: "pod config-hash differs from expected (spec change)",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v1"},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "expected-hash")),
			podsToUpgrade: []corev1.Pod{pod("masters-0", "masters", "different-hash", "old-trigger")},
			want:          false,
		},
		{
			name: "config hashes match, one pod has old restart-trigger",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v2"},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: []corev1.Pod{
				pod("masters-0", "masters", "hash1", "trigger-v1"),
				pod("masters-1", "masters", "hash1", "trigger-v2"),
			},
			want: true,
		},
		{
			name: "config hashes match, all pods have current restart-trigger",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v2"},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: []corev1.Pod{
				pod("masters-0", "masters", "hash1", "trigger-v2"),
				pod("masters-1", "masters", "hash1", "trigger-v2"),
			},
			want: false,
		},
		{
			name: "config hashes match, pod missing restart-trigger annotation",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v2"},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: []corev1.Pod{pod("masters-0", "masters", "hash1", "")},
			want:          true,
		},
		{
			name: "multiple StatefulSets, one pod has different config-hash",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v1"},
				},
			},
			resourcesList: resourcesList(
				stsWithHash("masters", "hash-m"),
				stsWithHash("data", "hash-d"),
			),
			podsToUpgrade: []corev1.Pod{
				pod("masters-0", "masters", "hash-m", "old"),
				pod("data-0", "data", "wrong-hash", "old"),
			},
			want: false,
		},
		{
			name: "multiple StatefulSets, config hashes match, one pod needs restart",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v2"},
				},
			},
			resourcesList: resourcesList(
				stsWithHash("masters", "hash-m"),
				stsWithHash("data", "hash-d"),
			),
			podsToUpgrade: []corev1.Pod{
				pod("masters-0", "masters", "hash-m", "trigger-v2"),
				pod("data-0", "data", "hash-d", "trigger-v1"),
			},
			want: true,
		},
		{
			name: "empty podsToUpgrade with restart annotation set",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v1"},
				},
			},
			resourcesList: resourcesList(stsWithHash("masters", "hash1")),
			podsToUpgrade: nil,
			want:          false,
		},
		{
			name: "StatefulSet without config-hash in template: pod with different hash still triggers spec-change false",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{esv1.RestartTriggerAnnotation: "trigger-v1"},
				},
			},
			resourcesList: resourcesList(sset.TestSset{Name: "masters", Namespace: "ns", Replicas: 1}.Build()),
			podsToUpgrade: []corev1.Pod{pod("masters-0", "masters", "some-hash", "old")},
			want:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAnnotationTriggeredRestart(tt.es, tt.resourcesList, tt.podsToUpgrade)
			assert.Equalf(t, tt.want, got, "isAnnotationTriggeredRestart() = %v, want %v", got, tt.want)
		})
	}
}
