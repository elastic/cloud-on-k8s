// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type testPod struct {
	name                                        string
	master, data, healthy, toUpgrade, inCluster bool
}
type upgradeTestPods []testPod

func newUpgradeTestPods(pods ...testPod) upgradeTestPods {
	result := make(upgradeTestPods, len(pods))
	for i := range pods {
		result[i] = pods[i]
	}
	return result
}

func (u upgradeTestPods) toPods() []runtime.Object {
	result := make([]runtime.Object, len(u))
	i := 0
	for _, testPod := range u {
		pod := testPod.toPod()
		result[i] = &pod
		i++
	}
	return result
}

func (u upgradeTestPods) toHealthyPods() map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, testPod := range u {
		pod := testPod.toPod()
		if k8s.IsPodReady(pod) {
			result[pod.Name] = pod
		}
	}
	return result
}

func (u upgradeTestPods) toUpgrade() []corev1.Pod {
	var result []corev1.Pod
	for _, testPod := range u {
		pod := testPod.toPod()
		if testPod.toUpgrade {
			result = append(result, pod)
		}
	}
	return result
}

func (u upgradeTestPods) inCluster() []string {
	var result []string
	for _, testPod := range u {
		pod := testPod.toPod()
		if testPod.inCluster {
			result = append(result, pod.Name)
		}
	}
	return result
}

func (u upgradeTestPods) toMasters() []string {
	var result []string
	for _, testPod := range u {
		pod := testPod.toPod()
		if label.IsMasterNode(pod) {
			result = append(result, pod.Name)
		}
	}
	return result
}

func names(pods []corev1.Pod) []string {
	result := make([]string, len(pods))
	for i, pod := range pods {
		result[i] = pod.Name
	}
	return result
}

func (t testPod) toPod() corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.name,
			Namespace: testNamespace,
		},
	}
	labels := map[string]string{}
	label.NodeTypesMasterLabelName.Set(t.master, labels)
	label.NodeTypesDataLabelName.Set(t.data, labels)
	pod.Labels = labels
	if t.healthy {
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
				return true, nil
			}
		}
	}
	return false, nil
}

func loadFileBytes(fileName string) []byte {
	contents, err := ioutil.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}

func (t *testESState) GetClusterState() (*esclient.ClusterState, error) {
	var cs esclient.ClusterState
	sampleClusterState := loadFileBytes("cluster_state.json")
	err := json.Unmarshal(sampleClusterState, &cs)
	return &cs, err
}

func TestDeletionStrategy_Predicates(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		ES              v1alpha1.Elasticsearch
		green           bool
	}
	type args struct {
		maxUnavailableReached bool
		allowedDeletions      int
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		deleted []string
		wantErr bool
	}{
		{
			name: "3 healthy master and data nodes, allow the last to be upgraded",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-2", true, true, true, false, true},
					testPod{"masters-1", true, true, true, false, true},
					testPod{"masters-0", true, true, true, true, true},
				),

				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{"masters-0"},
			wantErr: false,
		},
		{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-2", true, true, true, true, true},
					testPod{"masters-1", true, true, true, true, true},
					testPod{"masters-0", true, true, true, true, true},
				),
				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{"masters-2"},
			wantErr: false,
		},
		{
			name: "2 healthy masters out of 3, do not allow deletion",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, true, true, true, true},
					testPod{"masters-1", true, true, true, true, true},
					testPod{"masters-2", true, true, false, false, false},
				),

				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{},
			wantErr: false,
		},
		{
			name: "1 master and 1 node, wait for the node to be upgraded first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, false, true, true, true},
					testPod{"node-0", false, true, false, true, true},
				),
				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{"node-0"},
			wantErr: false,
		},
		{
			name: "Do not delete healthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, false, true, true, true},
				),
				green: false,
			},
			args: args{
				maxUnavailableReached: true,
				allowedDeletions:      1,
			},
			deleted: []string{},
			wantErr: false,
		},
		{
			name: "Allow deletion of unhealthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, true, true, true, true},
					testPod{"masters-1", true, true, true, true, true},
					testPod{"masters-2", true, true, false, true, true},
				),
				green: false,
			},
			args: args{
				maxUnavailableReached: true,
				allowedDeletions:      1,
			},
			deleted: []string{"masters-2"},
			wantErr: false,
		},
		{
			name: "Do not delete last healthy master",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, false, true, true, true},
					testPod{"masters-1", true, false, false, true, false},
				),
				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{"masters-1"},
			wantErr: false,
		},
		{
			name: "Do not delete Pods that share some shards",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"elasticsearch-sample-es-nodes-4", false, true, true, true, true},
					testPod{"elasticsearch-sample-es-nodes-3", false, true, true, true, true},
					testPod{"elasticsearch-sample-es-nodes-2", false, true, true, true, true},
					testPod{"elasticsearch-sample-es-nodes-1", false, true, true, true, true},
					testPod{"elasticsearch-sample-es-nodes-0", false, true, true, true, true},
					testPod{"elasticsearch-sample-es-masters-2", true, false, true, true, true},
					testPod{"elasticsearch-sample-es-masters-1", true, false, true, true, true},
					testPod{"elasticsearch-sample-es-masters-0", true, false, true, true, true},
				),
				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      2,
			},
			deleted: []string{"elasticsearch-sample-es-nodes-4", "elasticsearch-sample-es-nodes-2"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		esState := &testESState{
			inCluster: tt.fields.upgradeTestPods.inCluster(),
			green:     tt.fields.green,
		}
		ctx := NewPredicateContext(
			esState,
			tt.fields.upgradeTestPods.toHealthyPods(),
			tt.fields.upgradeTestPods.toUpgrade(),
			tt.fields.upgradeTestPods.toMasters(),
		)
		deleted, err := applyPredicates(ctx, tt.fields.upgradeTestPods.toUpgrade(), tt.args.maxUnavailableReached, tt.args.allowedDeletions)
		if (err != nil) != tt.wantErr {
			t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		assert.ElementsMatch(t, names(deleted), tt.deleted, tt.name)
	}
}

func TestDeletionStrategy_SortFunction(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		esState         ESState
	}
	tests := []struct {
		name   string
		fields fields
		want   []string // for this test we just compare the pod names
	}{
		{
			name: "Mixed nodes",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, true, true, true, true},
					testPod{"data-0", false, true, true, true, true},
					testPod{"masters-1", true, true, true, true, true},
					testPod{"data-1", false, true, true, true, true},
					testPod{"masters-2", true, true, false, true, true},
				),
				esState: &testESState{
					inCluster: []string{"data-1", "data-0", "masters-2", "masters-1", "masters-0"},
					green:     false,
				},
			},
			want: []string{"data-1", "data-0", "masters-2", "masters-1", "masters-0"},
		},
		{
			name: "Masters first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					testPod{"masters-0", true, true, true, true, true},
					testPod{"masters-1", true, true, true, true, true},
					testPod{"masters-2", true, true, false, true, true},
					testPod{"data-0", false, true, true, true, true},
					testPod{"data-1", false, true, true, true, true},
					testPod{"data-2", false, true, true, true, true},
				),
				esState: &testESState{
					inCluster: []string{"data-2", "data-1", "data-0", "masters-2", "masters-1", "masters-0"},
					green:     false,
				},
			},
			want: []string{"data-2", "data-1", "data-0", "masters-2", "masters-1", "masters-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toUpgrade := tt.fields.upgradeTestPods.toUpgrade()
			sortCandidates(toUpgrade)
			assert.Equal(t, len(tt.want), len(toUpgrade))
			for i := range tt.want {
				if tt.want[i] != toUpgrade[i].Name {
					t.Errorf("DeletionStrategyContext.SortFunction() = %v, want %v", names(toUpgrade), tt.want)
				}
			}
		})
	}
}
