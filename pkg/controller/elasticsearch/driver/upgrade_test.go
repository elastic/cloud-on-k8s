// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testNamespace = "ns"

func podWithRevision(name, revision string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    map[string]string{appsv1.StatefulSetRevisionLabel: revision},
		},
	}
}

func Test_podsToUpgrade(t *testing.T) {
	defaultEs := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			Version: "7.1.0",
		},
	}
	type args struct {
		pods         upgradeTestPods
		statefulSets sset.StatefulSetList
		es           esv1.Elasticsearch
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
						Name: "masters", Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 3},
					}.Build(),
				},
				pods: newUpgradeTestPods(
					newTestPod("masters-0").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("masters-1").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("nodes-0").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("nodes-1").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("nodes-2").withRevision("rev-a").withVersion("7.1.0"),
				),
				es: defaultEs,
			},
			want: []string{"masters-0", "masters-1", "nodes-0", "nodes-1", "nodes-2"},
		},
		{
			name: "only a sset needs to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: newUpgradeTestPods(
					newTestPod("masters-0").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("masters-1").withRevision("rev-a").withVersion("7.1.0"),
				),
				es: defaultEs,
			},
			want: []string{"masters-0", "masters-1"},
		},
		{
			name: "no pods to upgrade if the StatefulSet UpdateRevision is empty",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: newUpgradeTestPods(
					newTestPod("masters-0").withRevision("rev-a").withVersion("7.1.0"),
					newTestPod("masters-1").withRevision("rev-a").withVersion("7.1.0"),
				),
				es: defaultEs,
			},
			want: []string{},
		},
		{
			name: "only 1 node need to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b", UpdatedReplicas: 1, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b", UpdatedReplicas: 3, Replicas: 3},
					}.Build(),
				},
				pods: newUpgradeTestPods(
					newTestPod("masters-0").withRevision("rev-b").withVersion("7.1.0"),
					newTestPod("masters-1").withRevision("rev-a").withVersion("7.1.0"),
				),
				es: defaultEs,
			},
			want: []string{"masters-1"},
		},
		{
			name: "StatefulSet has been updated with a new ES version but StatefulSet update revision is not yet up to date",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{
						Name: "masters", Replicas: 2, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-a", UpdatedReplicas: 0, Replicas: 2},
					}.Build(),
					sset.TestSset{
						Name: "nodes", Replicas: 3, Master: true,
						Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-a", UpdatedReplicas: 0, Replicas: 3},
					}.Build(),
				},
				pods: newUpgradeTestPods(
					newTestPod("masters-0").withRevision("rev-a").withVersion("6.8.2"),
					newTestPod("masters-1").withRevision("rev-a").withVersion("6.8.2"),
					newTestPod("nodes-0").withRevision("rev-a").withVersion("6.8.2"),
					newTestPod("nodes-1").withRevision("rev-a").withVersion("6.8.2"),
					newTestPod("nodes-2").withRevision("rev-a").withVersion("6.8.2"),
				),
				es: defaultEs,
			},
			want: []string{"masters-0", "masters-1", "nodes-0", "nodes-1", "nodes-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.WrappedFakeClient(tt.args.pods.toRuntimeObjects(tt.args.es.Spec.Version, 1, nothing)...)
			got, err := podsToUpgrade(tt.args.es, client, tt.args.statefulSets)
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
					newTestPod("masters-2").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-0").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
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
					newTestPod("masters-2").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
					newTestPod("masters-1").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true).isTerminating(true),
					newTestPod("masters-0").inStatefulset("masters").isMaster(true).isData(false).isHealthy(true).needsUpgrade(true).isInCluster(true),
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
			client := k8s.WrappedFakeClient(tt.args.pods.toRuntimeObjects("7.5.0", 0, nothing)...)
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
