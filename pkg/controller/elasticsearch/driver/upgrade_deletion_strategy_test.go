// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newPod(namespace, name string, master, data, healthy bool) corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	labels := map[string]string{}
	label.NodeTypesMasterLabelName.Set(master, labels)
	label.NodeTypesDataLabelName.Set(data, labels)
	pod.Labels = labels
	if healthy {
		pod.Status = corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.PodReady,
					Status: corev1.ConditionTrue,
				},
				{
					Type:   corev1.ContainersReady,
					Status: corev1.ConditionTrue,
				},
			},
		}
	}
	return pod
}

type testESState struct {
	inCluster []string
	green     bool
	ESState
}

func (t *testESState) GreenHealth() (bool, error) {
	return t.green, nil
}

func (t *testESState) NodesInCluster(nodeNames []string) (bool, error) {
	for _, nodeName := range nodeNames {
		for _, inClusterPods := range t.inCluster {
			if nodeName == inClusterPods {
				continue
			}
			return false, nil
		}
	}
	return true, nil
}

func TestDeletionStrategy_Predicates(t *testing.T) {
	type fields struct {
		masterNodesNames []string
		healthyPods      map[string]corev1.Pod
		toUpdate         []corev1.Pod
		esState          ESState
	}
	type args struct {
		candidate             corev1.Pod
		deletedPods           []corev1.Pod
		maxUnavailableReached bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		deleted bool
		wantErr bool
	}{
		{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				masterNodesNames: []string{"masters-0", "masters-1", "masters-2"},
				healthyPods: map[string]corev1.Pod{
					"masters-0": newPod("ns1", "masters-0", true, true, true),
					"masters-1": newPod("ns1", "masters-1", true, true, true),
					"masters-2": newPod("ns1", "masters-2", true, true, true),
				},
				toUpdate: []corev1.Pod{
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, true),
				},
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1", "masters-2"},
					green:     true,
				},
			},
			args: args{
				candidate: newPod("ns1", "masters-2", true, true, true),
			},
			deleted: true,
			wantErr: false,
		},
		{
			name: "2 healthy masters out of 3, do not allow deletion",
			fields: fields{
				masterNodesNames: []string{"masters-0", "masters-1", "masters-2"},
				healthyPods: map[string]corev1.Pod{
					"masters-0": newPod("ns1", "masters-0", true, true, true),
					"masters-1": newPod("ns1", "masters-1", true, true, true),
				},
				toUpdate: []corev1.Pod{
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, false),
				},
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1"},
					green:     true,
				},
			},
			args: args{
				candidate: newPod("ns1", "masters-1", true, true, true),
			},
			deleted: false,
			wantErr: false,
		},
		{
			name: "1 master and 1 node, wait for the node to be upgraded first",
			fields: fields{
				masterNodesNames: []string{"masters-0"},
				healthyPods: map[string]corev1.Pod{
					"masters-0": newPod("ns1", "masters-0", true, true, true),
				},
				toUpdate: []corev1.Pod{
					newPod("ns1", "masters-0", true, false, true),
					newPod("ns1", "node-0", false, true, false),
				},
				esState: &testESState{
					inCluster: []string{"masters-0"},
					green:     true,
				},
			},
			args: args{
				candidate: newPod("ns1", "masters-0", true, true, true),
			},
			deleted: false,
			wantErr: false,
		},
		{
			name: "Do not delete healthy node if not green",
			fields: fields{
				masterNodesNames: []string{"masters-0"},
				healthyPods: map[string]corev1.Pod{
					"masters-0": newPod("ns1", "masters-0", true, true, true),
				},
				toUpdate: []corev1.Pod{
					newPod("ns1", "masters-0", true, true, true),
				},
				esState: &testESState{
					inCluster: []string{"masters-0"},
					green:     false,
				},
			},
			args: args{
				candidate: newPod("ns1", "masters-0", true, true, true),
			},
			deleted: false,
			wantErr: false,
		},
		{
			name: "Allow deletion of unhealthy node if not green",
			fields: fields{
				masterNodesNames: []string{"masters-0", "masters-1", "masters-2"},
				healthyPods: map[string]corev1.Pod{
					"masters-0": newPod("ns1", "masters-0", true, true, true),
					"masters-1": newPod("ns1", "masters-1", true, true, true),
				},
				toUpdate: []corev1.Pod{
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, false),
				},
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1"},
					green:     false,
				},
			},
			args: args{
				candidate: newPod("ns1", "masters-2", true, true, false),
			},
			deleted: true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		s := NewDefaultDeletionStrategy(tt.fields.esState, tt.fields.healthyPods, tt.fields.toUpdate, tt.fields.masterNodesNames)
		deleted, err := runPredicates(tt.args.candidate, tt.args.deletedPods, s.Predicates(), tt.args.maxUnavailableReached)
		if (err != nil) != tt.wantErr {
			t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		if deleted != tt.deleted {
			t.Errorf("name = %s, runPredicates = %v, want %v", tt.name, deleted, tt.deleted)
		}
	}
}
