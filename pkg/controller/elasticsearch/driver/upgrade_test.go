// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
					sset.TestSset{Name: "masters", Replicas: 2, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b"}}.Build(),
					sset.TestSset{Name: "nodes", Replicas: 3, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b"}}.Build(),
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
					sset.TestSset{Name: "masters", Replicas: 2, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b"}}.Build(),
					sset.TestSset{Name: "nodes", Replicas: 3, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b"}}.Build(),
				},
				pods: []runtime.Object{
					podWithRevision("masters-0", "rev-a"),
					podWithRevision("masters-1", "rev-a"),
				},
			},
			want: []string{"masters-0", "masters-1"},
		},
		{
			name: "only a1node need to be upgraded",
			args: args{
				statefulSets: sset.StatefulSetList{
					sset.TestSset{Name: "masters", Replicas: 2, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-a", UpdateRevision: "rev-b"}}.Build(),
					sset.TestSset{Name: "nodes", Replicas: 3, Master: true, Status: appsv1.StatefulSetStatus{CurrentRevision: "rev-b", UpdateRevision: "rev-b"}}.Build(),
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
			client := k8s.WrapClient(fake.NewFakeClient(tt.args.pods...))
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
					testPod{"masters-2", true, false, true, true, true},
					testPod{"masters-1", true, false, true, true, true},
					testPod{"masters-0", true, false, true, true, true},
				),
				statefulSets: sset.StatefulSetList{
					sset.TestSset{Name: "masters", Replicas: 3}.Build(),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esState := &testESState{
				inCluster: tt.args.pods.inCluster(),
			}
			client := k8s.WrapClient(fake.NewFakeClient(tt.args.pods.toPods()...))
			got, err := healthyPods(client, tt.args.statefulSets, esState)
			if (err != nil) != tt.wantErr {
				t.Errorf("healthyPods() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			want := tt.args.pods.toHealthyPods()
			if !reflect.DeepEqual(got, want) {
				t.Errorf("healthyPods() = %v, want %v", got, want)
			}
		})
	}
}
