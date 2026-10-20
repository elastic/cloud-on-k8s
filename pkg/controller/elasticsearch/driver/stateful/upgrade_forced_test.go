// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version/zen2"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_Driver_maybeForceUpgradePods(t *testing.T) {
	tests := []struct {
		name              string
		actualPods        []corev1.Pod
		podsToUpgrade     []corev1.Pod
		wantAttempted     bool
		wantRemainingPods []corev1.Pod
	}{
		{
			name: "no pods to upgrade",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetB", Ready: true, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: nil,
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetB", Ready: true, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "pods bootlooping in all StatefulSets, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "all pods bootlooping in StatefulSet B, upgrade them",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "all pods pending in all StatefulSets, upgrade them all",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name: "all pods pending in StatefulSet A, upgrade them",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "podA1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "podB1", StatefulSetName: "ssetB", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "podB2", StatefulSetName: "ssetB", Phase: corev1.PodRunning, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "1/2 pod to upgrade",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: true,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "at least one pod ready, don't upgrade any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: true, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, Phase: corev1.PodPending, ResourceVersion: "999"}.Build(),
			},
		},
		{
			name: "at least one pod not bootlooping, don't restart any",
			actualPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			podsToUpgrade: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
			wantAttempted: false,
			wantRemainingPods: []corev1.Pod{
				sset.TestPod{Name: "pod1", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod2", StatefulSetName: "ssetA", Ready: false, RestartCount: 0, ResourceVersion: "999"}.Build(),
				sset.TestPod{Name: "pod3", StatefulSetName: "ssetA", Ready: false, RestartCount: 1, ResourceVersion: "999"}.Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeObjs := make([]crclient.Object, 0, len(tt.actualPods))
			for i := range tt.actualPods {
				runtimeObjs = append(runtimeObjs, &tt.actualPods[i])
			}
			k8sClient := k8s.NewFakeClient(runtimeObjs...)
			d := &Driver{

				Client:         k8sClient,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(esv1.Elasticsearch{}),
			}

			attempted, err := d.maybeForceUpgradePods(context.Background(), tt.actualPods, tt.podsToUpgrade)
			require.NoError(t, err)
			require.Equal(t, tt.wantAttempted, attempted)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &pods))
			require.ElementsMatch(t, tt.wantRemainingPods, pods.Items)
		})
	}
}

// Test_Driver_MaybeForceUpgrade_leavingPods checks that a forced upgrade does not delete the leaving Pods that are
// draining, but still deletes the leaving Pods that have no shutdown to protect, as they may be needed to recover. All
// the Pods are still considered to decide whether a StatefulSet can be force upgraded.
func Test_Driver_MaybeForceUpgrade_leavingPods(t *testing.T) {
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0}
	statefulSet := func(name string, replicas int32, master bool) appsv1.StatefulSet {
		status := outdated
		status.Replicas = replicas
		return sset.TestSset{Name: name, Namespace: TestEsNamespace, ClusterName: TestEsName, Replicas: replicas, Master: master, Data: !master, Status: status}.Build()
	}
	// the Pods are outdated. They are Pending if they cannot be scheduled, otherwise bootlooping unless they are ready.
	pod := func(statefulSet appsv1.StatefulSet, ordinal int32, pending, ready bool) corev1.Pod {
		testPod := sset.TestPod{
			Name: sset.PodName(statefulSet.Name, ordinal), Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: statefulSet.Name,
			Revision: "rev-a", Master: label.IsMasterNodeSet(statefulSet), Data: !label.IsMasterNodeSet(statefulSet), Ready: ready, RestartCount: 1,
		}
		if pending {
			testPod.Phase, testPod.RestartCount = corev1.PodPending, 0
		}
		return testPod.Build()
	}
	tests := []struct {
		name      string
		actual    es_sset.StatefulSetList
		expected  es_sset.StatefulSetList
		pending   bool
		readyPods []string
		// runningPods are Pending Pods whose Elasticsearch container is running
		runningPods []string
		// esReachable makes the shutdown interface available
		esReachable bool
		// shutdowns are the nodes that are members of the cluster with a remove-type shutdown
		shutdowns []string
		// unreachedShutdowns are the nodes with a remove-type shutdown that are missing from a partial nodes response
		unreachedShutdowns []string
		// reportedShutdowns are the nodes with a shutdown in the downscale status, the other leaving nodes are reported as
		// not started
		reportedShutdowns []string
		// initialMasterNodes are the masters the zen2 bootstrap of the cluster still requires
		initialMasterNodes []string
		getShutdownErr     error
		wantAttempted      bool
		wantErr            bool
		wantLookups        int
		wantRemainingPods  []string
	}{
		{
			name:              "draining Pods of a StatefulSet that is not expected anymore are not deleted",
			actual:            es_sset.StatefulSetList{statefulSet("data", 1, false), statefulSet("data-new", 1, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data-new", 1, false)},
			esReachable:       true,
			shutdowns:         []string{"data-0"},
			wantAttempted:     true,
			wantLookups:       1,
			wantRemainingPods: []string{"data-0"},
		},
		{
			name:              "draining Pods above the expected number of replicas are not deleted",
			actual:            es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data", 1, false)},
			esReachable:       true,
			shutdowns:         []string{"data-1"},
			wantAttempted:     true,
			wantLookups:       1,
			wantRemainingPods: []string{"data-1"},
		},
		{
			name:          "leaving Pods without a shutdown are deleted",
			actual:        es_sset.StatefulSetList{statefulSet("data", 3, false)},
			expected:      es_sset.StatefulSetList{statefulSet("data", 1, false)},
			esReachable:   true,
			shutdowns:     []string{"data-2"},
			wantAttempted: true,
			wantLookups:   1,
			// data-1 has no shutdown to protect
			wantRemainingPods: []string{"data-2"},
		},
		{
			name:          "Pending data Pods repaired while reduced from 3 to 2 are all recreated",
			actual:        es_sset.StatefulSetList{statefulSet("data", 3, false)},
			expected:      es_sset.StatefulSetList{statefulSet("data", 2, false)},
			pending:       true,
			esReachable:   true,
			wantAttempted: true,
			// data-2 never joined the cluster: it must be recreated with the repaired template to join it and be removed
			wantLookups:       0,
			wantRemainingPods: nil,
		},
		{
			name:     "Pending masters repaired while reduced from 3 to 1 are all recreated",
			actual:   es_sset.StatefulSetList{statefulSet("master", 3, true)},
			expected: es_sset.StatefulSetList{statefulSet("master", 1, true)},
			pending:  true,
			// the cluster never formed, the bootstrap still requires the 3 masters to elect a master
			initialMasterNodes: []string{"master-0", "master-1", "master-2"},
			wantAttempted:      true,
			wantRemainingPods:  nil,
		},
		{
			name:     "bootlooping masters repaired while reduced from 3 to 1 are all recreated while Elasticsearch is unreachable",
			actual:   es_sset.StatefulSetList{statefulSet("master", 3, true)},
			expected: es_sset.StatefulSetList{statefulSet("master", 1, true)},
			// the voting configuration still requires 2 of the 3 masters
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name:     "draining Pods are not deleted while Elasticsearch is unreachable, based on the reported shutdowns",
			actual:   es_sset.StatefulSetList{statefulSet("data", 3, false)},
			expected: es_sset.StatefulSetList{statefulSet("data", 1, false)},
			// a transient outage does not stop the shutdown of data-2, data-1 has none
			reportedShutdowns: []string{"data-2"},
			wantAttempted:     true,
			wantRemainingPods: []string{"data-2"},
		},
		{
			name:              "a Pending draining Pod is deleted, as no Elasticsearch node runs in it",
			actual:            es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data", 1, false)},
			pending:           true,
			reportedShutdowns: []string{"data-1"},
			wantAttempted:     true,
			wantRemainingPods: nil,
		},
		{
			name:     "a Pending draining Pod is not deleted if Elasticsearch runs in it",
			actual:   es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected: es_sset.StatefulSetList{statefulSet("data", 1, false)},
			pending:  true,
			// another container of data-1 waits for its image, Elasticsearch is running and draining
			runningPods:       []string{"data-1"},
			esReachable:       true,
			shutdowns:         []string{"data-1"},
			wantAttempted:     true,
			wantLookups:       1,
			wantRemainingPods: []string{"data-1"},
		},
		{
			// data-1 is a draining member that did not respond to the nodes request, so it has no node ID
			name:               "a draining Pod missing from a partial nodes response with a reported shutdown is not deleted",
			actual:             es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:           es_sset.StatefulSetList{statefulSet("data", 1, false)},
			esReachable:        true,
			unreachedShutdowns: []string{"data-1"},
			reportedShutdowns:  []string{"data-1"},
			wantAttempted:      true,
			wantLookups:        1,
			wantRemainingPods:  []string{"data-1"},
		},
		{
			name:               "a Pod missing from a partial nodes response without a reported shutdown is deleted",
			actual:             es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:           es_sset.StatefulSetList{statefulSet("data", 1, false)},
			esReachable:        true,
			unreachedShutdowns: []string{"data-1"},
			wantAttempted:      true,
			wantLookups:        1,
			wantRemainingPods:  nil,
		},
		{
			name:              "a StatefulSet that cannot be force upgraded is left untouched",
			actual:            es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data", 1, false)},
			readyPods:         []string{"data-0"},
			esReachable:       true,
			wantAttempted:     false,
			wantLookups:       1,
			wantRemainingPods: []string{"data-0", "data-1"},
		},
		{
			name:              "shutdowns are not retrieved if no Pod to force upgrade is leaving",
			actual:            es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data", 2, false)},
			esReachable:       true,
			wantAttempted:     true,
			wantLookups:       0,
			wantRemainingPods: nil,
		},
		{
			name:              "an error while retrieving the shutdowns is returned, no Pod is deleted",
			actual:            es_sset.StatefulSetList{statefulSet("data", 2, false)},
			expected:          es_sset.StatefulSetList{statefulSet("data", 1, false)},
			esReachable:       true,
			getShutdownErr:    errors.New("boom"),
			wantErr:           true,
			wantLookups:       1,
			wantRemainingPods: []string{"data-0", "data-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			desiredLeaving := desiredLeavingNodeNames(ctx, tt.expected, tt.actual)
			es := esv1.Elasticsearch{Name: TestEsName, Namespace: TestEsNamespace}
			if len(tt.initialMasterNodes) > 0 {
				es.Annotations = map[string]string{zen2.InitialMasterNodesAnnotation: strings.Join(tt.initialMasterNodes, ",")}
			}
			for _, node := range desiredLeaving.AsSortedSlice() {
				status := string(esclient.ShutdownNotStarted)
				if slices.Contains(tt.reportedShutdowns, node) {
					status = string(esclient.ShutdownInProgress)
				}
				es.Status.DownscaleOperation.Nodes = append(es.Status.DownscaleOperation.Nodes, esv1.DownscaledNode{Name: node, ShutdownStatus: status})
			}
			objs := []crclient.Object{&es}
			for _, s := range tt.actual {
				for i := range sset.GetReplicas(s) {
					p := pod(s, i, tt.pending, slices.Contains(tt.readyPods, sset.PodName(s.Name, i)))
					if slices.Contains(tt.runningPods, p.Name) {
						p.Status.ContainerStatuses[0].State.Running = &corev1.ContainerStateRunning{}
					}
					objs = append(objs, &p)
				}
			}
			k8sClient := k8s.NewFakeClient(objs...)
			d := &Driver{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}
			esClient := &fakeESClient{
				version:        version.MustParse("8.15.0"),
				nodes:          esclient.Nodes{Nodes: map[string]esclient.Node{}},
				Shutdowns:      map[string]esclient.NodeShutdown{},
				GetShutdownErr: tt.getShutdownErr,
			}
			for _, node := range tt.shutdowns {
				esClient.nodes.Nodes["id-"+node] = esclient.Node{Name: node}
				esClient.Shutdowns["id-"+node] = esclient.NodeShutdown{NodeID: "id-" + node, Type: string(esclient.Remove), Status: esclient.ShutdownInProgress}
			}
			if len(tt.unreachedShutdowns) > 0 {
				esClient.nodes.Summary.Failed = len(tt.unreachedShutdowns)
			}
			for _, node := range tt.unreachedShutdowns {
				esClient.Shutdowns["id-"+node] = esclient.NodeShutdown{NodeID: "id-" + node, Type: string(esclient.Remove), Status: esclient.ShutdownInProgress}
			}
			// the shutdown interface is only available if Elasticsearch is reachable
			var nodeShutdowns shutdown.Interface
			var counting *countingNodeShutdown
			if tt.esReachable {
				ns, err := newShutdownInterface(ctx, es, esClient, NewMemoizingESState(ctx, esClient), nil)
				require.NoError(t, err)
				counting = &countingNodeShutdown{Interface: ns}
				nodeShutdowns = counting
			}

			attempted, err := d.MaybeForceUpgrade(ctx, tt.actual, newLeavingNodes(desiredLeaving, NewMemoizingESState(ctx, esClient), nodeShutdowns, es.Status.DownscaleOperation))
			require.Equal(t, tt.wantErr, err != nil, err)
			require.Equal(t, tt.wantAttempted, attempted)
			var lookups int
			if counting != nil {
				lookups = counting.nodesWithShutdownCalls
			}
			require.Equal(t, tt.wantLookups, lookups)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(ctx, &pods))
			require.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))

			if len(tt.initialMasterNodes) > 0 {
				// the masters are recreated with the bootstrap of all the initial masters, even if fewer are expected
				resources := nodespec.ResourcesList{{StatefulSet: tt.expected[0], Config: settings.NewCanonicalConfig()}}
				require.NoError(t, zen2.SetupInitialMasterNodes(ctx, es, k8sClient, resources))
				expected, err := settings.CanonicalConfig{CanonicalConfig: commonsettings.MustCanonicalConfig(map[string][]string{
					esv1.ClusterInitialMasterNodes: tt.initialMasterNodes,
				})}.Render()
				require.NoError(t, err)
				actual, err := resources[0].Config.Render()
				require.NoError(t, err)
				require.Equal(t, string(expected), string(actual))
			}
		})
	}
}
