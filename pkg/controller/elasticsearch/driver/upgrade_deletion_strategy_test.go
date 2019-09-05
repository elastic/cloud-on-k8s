// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	sampleClusterState = `{
  "cluster_name" : "elasticsearch-sample",
  "cluster_uuid" : "n3tTyyoyTlqsZ0xMwekFWw",
  "master_node" : "GA-ZgR0bRC64iPAoXRAwng",
  "nodes" : {
    "J_D32LThRSW8EOsDQ8al0A" : {
      "name" : "elasticsearch-sample-es-nodes-1",
      "ephemeral_id" : "dUXjWNtBSDOYEl7vEAv3hw",
      "transport_address" : "10.233.66.157:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "n9MS8Bn8R1m-T9u76wU45w" : {
      "name" : "elasticsearch-sample-es-masters-0",
      "ephemeral_id" : "_MoCcpNMRDy2PCbDjV4P6w",
      "transport_address" : "10.233.65.34:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "lwr67QTrRdqTGfD_bN2tPQ" : {
      "name" : "elasticsearch-sample-es-masters-1",
      "ephemeral_id" : "767pD01ZQAyS9PcTg6YixA",
      "transport_address" : "10.233.66.158:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "CBOVABG9QNGLGh1w23UGsg" : {
      "name" : "elasticsearch-sample-es-nodes-3",
      "ephemeral_id" : "nxr1tnJCReCJGv6t3rvakA",
      "transport_address" : "10.233.65.58:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "ZRz1d_mLQq-GbY-ceYaGuQ" : {
      "name" : "elasticsearch-sample-es-nodes-0",
      "ephemeral_id" : "tp9l5jWXTZC_r3k1PbjtUg",
      "transport_address" : "10.233.65.46:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "GA-ZgR0bRC64iPAoXRAwng" : {
      "name" : "elasticsearch-sample-es-masters-2",
      "ephemeral_id" : "6kV_nWmPSFO_whRL2IPV4Q",
      "transport_address" : "10.233.67.124:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "DeyNLGZBSM6jLaExq4glzQ" : {
      "name" : "elasticsearch-sample-es-nodes-2",
      "ephemeral_id" : "__kzR3lCTwamotedejJMmA",
      "transport_address" : "10.233.67.15:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "lgiq13sPRVWlBiPTLaTivA" : {
      "name" : "elasticsearch-sample-es-nodes-4",
      "ephemeral_id" : "YrXGoHfkS9KQLYhZbAxcQw",
      "transport_address" : "10.233.66.142:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    }
  },
  "routing_table" : {
    "indices" : {
      ".security-7" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : false,
              "node" : "ZRz1d_mLQq-GbY-ceYaGuQ",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".security-7",
              "allocation_id" : {
                "id" : "7mnstP8WSUahLLTHuEvVoA"
              }
            },
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "J_D32LThRSW8EOsDQ8al0A",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".security-7",
              "allocation_id" : {
                "id" : "xKxkOUGsRMe0XiLDhq-w3g"
              }
            }
          ]
        }
      },
      "twitter" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "DeyNLGZBSM6jLaExq4glzQ",
              "relocating_node" : null,
              "shard" : 0,
              "index" : "twitter",
              "allocation_id" : {
                "id" : "5YJuiwmVTMu9r13o4s_Ziw"
              }
            }
          ]
        }
      },
      ".kibana_1" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "CBOVABG9QNGLGh1w23UGsg",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".kibana_1",
              "allocation_id" : {
                "id" : "VFC99m2RSXmPbUS6U_nR0Q"
              }
            },
            {
              "state" : "STARTED",
              "primary" : false,
              "node" : "lgiq13sPRVWlBiPTLaTivA",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".kibana_1",
              "allocation_id" : {
                "id" : "6fAfn-uITu6yYQ6zsGB97A"
              }
            }
          ]
        }
      }
    }
  }
}
`
)

type testPod struct {
	namespace, name                             string
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

func (u upgradeTestPods) toHealthyPods() map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, testPod := range u {
		pod := testPod.toPod()
		if k8s.IsPodReady(pod) {
			pod := pod
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

func newTestPod(namespace, name string, master, data, healthy, toUpgrade, inCluster bool) testPod {
	return testPod{
		namespace: namespace,
		name:      name,
		master:    master,
		data:      data,
		healthy:   healthy,
		toUpgrade: toUpgrade,
		inCluster: inCluster,
	}
}

func (t testPod) toPod() corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      t.name,
			Namespace: t.namespace,
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
				continue
			}
			return false, nil
		}
	}
	return true, nil
}

func (t *testESState) GetClusterState() (*esclient.ClusterState, error) {
	var cs esclient.ClusterState
	err := json.Unmarshal([]byte(sampleClusterState), &cs)
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
		/*{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("ns1", "masters-2", true, true, true, true, true),
					newTestPod("ns1", "masters-1", true, true, true, true, true),
					newTestPod("ns1", "masters-0", true, true, true, true, true),
				),
				green: true,
			},
			args: args{
				maxUnavailableReached: false,
				allowedDeletions:      1,
			},
			deleted: []string{"masters-2"},
			wantErr: false,
		},*/
		{
			name: "2 healthy masters out of 3, do not allow deletion",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newTestPod("ns1", "masters-0", true, true, true, true, true),
					newTestPod("ns1", "masters-1", true, true, true, true, true),
					newTestPod("ns1", "masters-2", true, true, false, false, false),
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
					newTestPod("ns1", "masters-0", true, false, true, true, true),
					newTestPod("ns1", "node-0", false, true, false, true, true),
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
					newTestPod("ns1", "masters-0", true, false, true, true, true),
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
					newTestPod("ns1", "masters-0", true, true, true, true, true),
					newTestPod("ns1", "masters-1", true, true, true, true, true),
					newTestPod("ns1", "masters-2", true, true, false, true, true),
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
					newTestPod("ns1", "masters-0", true, false, true, true, true),
					newTestPod("ns1", "masters-1", true, false, false, true, false),
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
					newTestPod("ns1", "elasticsearch-sample-es-nodes-4", false, true, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-nodes-3", false, true, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-nodes-2", false, true, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-nodes-1", false, true, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-nodes-0", false, true, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-masters-2", true, false, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-masters-1", true, false, true, true, true),
					newTestPod("ns1", "elasticsearch-sample-es-masters-0", true, false, true, true, true),
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
					newTestPod("ns1", "masters-0", true, true, true, true, true),
					newTestPod("ns1", "data-0", false, true, true, true, true),
					newTestPod("ns1", "masters-1", true, true, true, true, true),
					newTestPod("ns1", "data-1", false, true, true, true, true),
					newTestPod("ns1", "masters-2", true, true, false, true, true),
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
					newTestPod("ns1", "masters-0", true, true, true, true, true),
					newTestPod("ns1", "masters-1", true, true, true, true, true),
					newTestPod("ns1", "masters-2", true, true, false, true, true),
					newTestPod("ns1", "data-0", false, true, true, true, true),
					newTestPod("ns1", "data-1", false, true, true, true, true),
					newTestPod("ns1", "data-2", false, true, true, true, true),
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
