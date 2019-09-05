// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"encoding/json"
	"testing"

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

type upgradeTestPods []corev1.Pod

func newUpgradeTestPods(pods ...corev1.Pod) upgradeTestPods {
	result := make(upgradeTestPods, len(pods))
	for i := range pods {
		result[i] = pods[i]
	}
	return result
}

func (u upgradeTestPods) toHealthyPods() map[string]corev1.Pod {
	result := make(map[string]corev1.Pod)
	for _, pod := range u {
		if k8s.IsPodReady(pod) {
			pod := pod
			result[pod.Name] = pod
		}
	}
	return result
}

func (u upgradeTestPods) toUpgrade() []corev1.Pod {
	var result []corev1.Pod
	for _, pod := range u {
		pod := pod
		result = append(result, pod)
	}
	return result
}

func (u upgradeTestPods) toMasters() []string {
	var result []string
	for _, pod := range u {
		if label.IsMasterNode(pod) {
			pod := pod
			result = append(result, pod.Name)
		}
	}
	return result
}

func names(pods []corev1.Pod) []string {
	var result []string
	for _, pod := range pods {
		result = append(result, pod.Name)
	}
	return result
}

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

func (t *testESState) GetClusterState() (*esclient.ClusterState, error) {
	var cs esclient.ClusterState
	err := json.Unmarshal([]byte(sampleClusterState), &cs)
	return &cs, err
}

func TestDeletionStrategy_Predicates(t *testing.T) {
	type fields struct {
		upgradeTestPods upgradeTestPods
		esState         ESState
	}
	type args struct {
		candidates            []corev1.Pod
		maxUnavailableReached bool
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		deleted []bool
		wantErr bool
	}{
		{
			name: "3 healthy masters, allow the deletion of 1",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, true),
				),
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1", "masters-2"},
					green:     true,
				},
			},
			args: args{
				candidates: []corev1.Pod{newPod("ns1", "masters-2", true, true, true)},
			},
			deleted: []bool{true},
			wantErr: false,
		},
		{
			name: "2 healthy masters out of 3, do not allow deletion",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, false),
				),
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1"},
					green:     true,
				},
			},
			args: args{
				candidates: []corev1.Pod{newPod("ns1", "masters-1", true, true, true)},
			},
			deleted: []bool{false},
			wantErr: false,
		},
		{
			name: "1 master and 1 node, wait for the node to be upgraded first",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "masters-0", true, false, true),
					newPod("ns1", "node-0", false, true, false),
				),
				esState: &testESState{
					inCluster: []string{"masters-0"},
					green:     true,
				},
			},
			args: args{
				candidates: []corev1.Pod{newPod("ns1", "masters-0", true, true, true)},
			},
			deleted: []bool{false},
			wantErr: false,
		},
		{
			name: "Do not delete healthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "masters-0", true, false, true),
				),
				esState: &testESState{
					inCluster: []string{"masters-0"},
					green:     false,
				},
			},
			args: args{
				candidates: []corev1.Pod{newPod("ns1", "masters-0", true, true, true)},
			},
			deleted: []bool{false},
			wantErr: false,
		},
		{
			name: "Allow deletion of unhealthy node if not green",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, false),
				),
				esState: &testESState{
					inCluster: []string{"masters-0", "masters-1"},
					green:     false,
				},
			},
			args: args{
				candidates: []corev1.Pod{newPod("ns1", "masters-2", true, true, false)},
			},
			deleted: []bool{true},
			wantErr: false,
		},
		{
			name: "Do not delete Pods that share some shards",
			fields: fields{
				upgradeTestPods: newUpgradeTestPods(
					newPod("ns1", "elasticsearch-sample-es-masters-0", true, false, true),
					newPod("ns1", "elasticsearch-sample-es-masters-1", true, false, true),
					newPod("ns1", "elasticsearch-sample-es-masters-3", true, false, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-0", false, true, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-1", false, true, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-2", false, true, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-3", false, true, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-4", false, true, true),
				),
				esState: &testESState{
					inCluster: []string{
						"elasticsearch-sample-es-masters-0", "elasticsearch-sample-es-masters-1", "elasticsearch-sample-es-masters-3",
						"elasticsearch-sample-es-nodes-0", "elasticsearch-sample-es-nodes-1", "elasticsearch-sample-es-nodes-2",
						"elasticsearch-sample-es-nodes-3", "elasticsearch-sample-es-nodes-4"},
					green: true,
				},
			},
			args: args{
				candidates: []corev1.Pod{
					newPod("ns1", "elasticsearch-sample-es-nodes-4", false, true, true),
					newPod("ns1", "elasticsearch-sample-es-nodes-3", false, true, true),
				},
			},
			deleted: []bool{true, false},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		ctx := NewPredicateContext(
			tt.fields.esState,
			tt.fields.upgradeTestPods.toHealthyPods(),
			tt.fields.upgradeTestPods.toUpgrade(),
			tt.fields.upgradeTestPods.toMasters(),
		)
		var deletedPods []corev1.Pod
		for i, candidate := range tt.args.candidates {
			deleted, err := runPredicates(ctx, candidate, deletedPods, tt.args.maxUnavailableReached)
			if (err != nil) != tt.wantErr {
				t.Errorf("runPredicates error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if deleted != tt.deleted[i] {
				t.Errorf("name = %s, runPredicates = %v, want %v", tt.name, deleted, tt.deleted)
			}
			if deleted {
				deletedPods = append(deletedPods, candidate)
			}
		}
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
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "data-0", false, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "data-1", false, true, true),
					newPod("ns1", "masters-2", true, true, false),
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
					newPod("ns1", "masters-0", true, true, true),
					newPod("ns1", "masters-1", true, true, true),
					newPod("ns1", "masters-2", true, true, false),
					newPod("ns1", "data-0", false, true, true),
					newPod("ns1", "data-1", false, true, true),
					newPod("ns1", "data-2", false, true, true),
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
