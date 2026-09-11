// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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

func Test_podsToRollingUpgrade(t *testing.T) {
	type args struct {
		pods                 []client.Object
		statefulSets         es_sset.StatefulSetList
		expectedStatefulSets es_sset.StatefulSetList
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
			name: "pods of a StatefulSet that is not expected anymore are not upgraded",
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
			},
			want: []string{"data-new-0", "data-new-1"},
		},
		{
			name: "pods whose ordinal is at or above the expected number of replicas are not upgraded",
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
			want: []string{"data-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.args.pods...)
			got, err := podsToRollingUpgrade(context.Background(), client, tt.args.expectedStatefulSets, tt.args.statefulSets)
			require.NoError(t, err)
			assert.ElementsMatch(t, names(got), tt.want, tt.name)
		})
	}
}

// Test_Driver_handleUpgrades_leavingPodsAreNotRestarted drives handleUpgrades with a nodeSet that has been removed from the
// spec while a rolling upgrade was still pending on it: its Pod must be left to the downscale, while the outdated Pod of a
// nodeSet that stays is still upgraded. Each case has exactly one outdated Pod so that the outcome does not depend on the
// order in which candidates are considered.
func Test_Driver_handleUpgrades_leavingPodsAreNotRestarted(t *testing.T) {
	esVersion := "8.1.0"
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: TestEsName, Namespace: TestEsNamespace},
		Spec:       esv1.ElasticsearchSpec{Version: esVersion},
		Status:     esv1.ElasticsearchStatus{Version: esVersion},
	}
	outdated := appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 1}
	upToDate := appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 1, Replicas: 1}
	statefulSet := func(name string, status appsv1.StatefulSetStatus) appsv1.StatefulSet {
		return sset.TestSset{Name: name, Namespace: TestEsNamespace, ClusterName: TestEsName, Version: esVersion, Replicas: 1, Data: true, Status: status}.Build()
	}
	pod := func(ssetName, revision string) corev1.Pod {
		return sset.TestPod{Name: ssetName + "-0", Namespace: TestEsNamespace, ClusterName: TestEsName, StatefulSetName: ssetName, Version: esVersion, Revision: revision, Data: true, Ready: true}.Build()
	}
	tests := []struct {
		name              string
		leaving           appsv1.StatefulSetStatus // status of "data", which is absent from the expected StatefulSets
		staying           appsv1.StatefulSetStatus // status of "data-new", which is expected
		wantShutdowns     []shutdownCall           // shutdowns requested, by node ID and type
		wantRemainingPods []string                 // Pods that have not been deleted for an upgrade
	}{
		{
			name:              "only the leaving Pod is outdated: nothing is restarted",
			leaving:           outdated,
			staying:           upToDate,
			wantShutdowns:     nil,
			wantRemainingPods: []string{"data-0", "data-new-0"},
		},
		{
			name:              "only the staying Pod is outdated: it is restarted",
			leaving:           upToDate,
			staying:           outdated,
			wantShutdowns:     []shutdownCall{{NodeID: "id-data-new-0", Type: esclient.Restart}},
			wantRemainingPods: []string{"data-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leavingSset, stayingSset := statefulSet("data", tt.leaving), statefulSet("data-new", tt.staying)
			leavingPod, stayingPod := pod("data", tt.leaving.CurrentRevision), pod("data-new", tt.staying.CurrentRevision)
			k8sClient := k8s.NewFakeClient(&es, &leavingSset, &stayingSset, &leavingPod, &stayingPod)
			esClient := &fakeESClient{
				version: version.MustParse(esVersion),
				nodes:   esclient.Nodes{Nodes: map[string]esclient.Node{"id-data-0": {Name: "data-0"}, "id-data-new-0": {Name: "data-new-0"}}},
				health:  esclient.Health{Status: esv1.ElasticsearchGreenHealth},
			}
			d := &Driver{BaseDriver: driver.BaseDriver{Parameters: driver.Parameters{
				Client:         k8sClient,
				ES:             es,
				Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				ReconcileState: reconcile.MustNewState(es),
			}}}
			// the spec only has data-new left: data is being removed
			expectedResources := nodespec.ResourcesList{{StatefulSet: stayingSset}}

			results := d.handleUpgrades(context.Background(), esClient, NewMemoizingESState(context.Background(), esClient), expectedResources)

			_, err := results.Aggregate()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantShutdowns, esClient.PutShutdownCalls)
			var pods corev1.PodList
			require.NoError(t, k8sClient.List(context.Background(), &pods))
			assert.ElementsMatch(t, tt.wantRemainingPods, names(pods.Items))
		})
	}
}

// Test_Driver_handleUpgrades_leavingPodsSurviveFullClusterRestart covers the other deletion branch of handleUpgrades: a
// version upgrade of a non-HA cluster restarts all outdated Pods at once, without predicates. A Pod of a nodeSet that
// is being removed must not be part of that restart either.
func Test_Driver_handleUpgrades_leavingPodsSurviveFullClusterRestart(t *testing.T) {
	fromVersion, toVersion := "8.1.0", "8.2.0"
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: TestEsName, Namespace: TestEsNamespace},
		Spec:       esv1.ElasticsearchSpec{Version: toVersion},
		Status:     esv1.ElasticsearchStatus{Version: fromVersion},
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
	}
	d := &Driver{BaseDriver: driver.BaseDriver{Parameters: driver.Parameters{
		Client:         k8sClient,
		ES:             es,
		Expectations:   expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
		ReconcileState: reconcile.MustNewState(es),
	}}}
	expectedResources := nodespec.ResourcesList{{StatefulSet: masterSset}}
	// make sure the fixture really selects the full-restart branch
	currentPods, err := es_sset.StatefulSetList{masterSset, leavingSset}.GetActualPods(k8sClient)
	require.NoError(t, err)
	isUpgrade, err := isVersionUpgrade(es)
	require.NoError(t, err)
	require.True(t, isUpgrade)
	require.True(t, isNonHACluster(currentPods, expectedResources.MasterNodesNames()))

	results := d.handleUpgrades(context.Background(), esClient, NewMemoizingESState(context.Background(), esClient), expectedResources)

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

			d := &Driver{
				BaseDriver: driver.BaseDriver{Parameters: driver.Parameters{
					Client:         client,
					ES:             tt.es,
					Expectations:   expectations.NewExpectations(client, &appsv1.StatefulSet{}),
					ReconcileState: reconcileState,
				}},
			}
			if tt.expectations != nil {
				tt.expectations(d.Expectations)
			}

			nodeNameToID, err := esState.NodeNameToID()
			require.NoError(t, err)

			n := shutdown.NewNodeShutdown(esClient, nodeNameToID, esclient.Restart, "", nil, crlog.Log)
			results := d.maybeCompleteNodeUpgrades(context.Background(), esClient, esState, n)
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
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: tt.resourceVersion,
					Annotations:     tt.annotations,
				},
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
