// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cluster

import (
	"encoding/json"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ClusterStateSample = `
{
    "cluster_name": "elasticsearch-sample",
    "compressed_size_in_bytes": 10281,
    "cluster_uuid": "fW1CurdKQpa-vsEYgTwkvg",
    "version": 28,
    "state_uuid": "0_7Tkm3ERdeB5eOqEgdOcA",
    "master_node": "EizpW8QWRty_T1nJpr-dNQ",
    "nodes": {
        "EizpW8QWRty_T1nJpr-dNQ": {
            "name": "elasticsearch-sample-es-fnsgkkdl85",
            "ephemeral_id": "hd8VlWVdTlyCriXKDW-5kg",
            "transport_address": "172.17.0.10:9300",
            "attributes": {
                "xpack.installed": "true"
            }
        },
        "NRqCLTmhTLuSxzlWcTae3A": {
            "name": "elasticsearch-sample-es-79gc6p57rs",
            "ephemeral_id": "VHAy3TOxTby3fNaPpMgfkg",
            "transport_address": "172.17.0.9:9300",
            "attributes": {
                "xpack.installed": "true"
            }
        },
        "q--ANfDnTKW2WS9pEBuLWQ": {
            "name": "elasticsearch-sample-es-jfpqbt2s4q",
            "ephemeral_id": "USglep8YTW-4vZ9M7PyRqA",
            "transport_address": "172.17.0.7:9300",
            "attributes": {
                "xpack.installed": "true"
            }
        }
    },
    "routing_table": {
        "indices": {
            "shakespeare": {
                "shards": {
                    "0": [
                        {
                            "state": "STARTED",
                            "primary": true,
                            "node": "q--ANfDnTKW2WS9pEBuLWQ",
                            "relocating_node": null,
                            "shard": 0,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "TtAx_PMwRCmanPR7XddWmg"
                            }
                        },
                        {
                            "state": "STARTED",
                            "primary": false,
                            "node": "EizpW8QWRty_T1nJpr-dNQ",
                            "relocating_node": null,
                            "shard": 0,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "QddiDZTHTuStDTIKSOIk5A"
                            }
                        }
                    ],
                    "1": [
                        {
                            "state": "STARTED",
                            "primary": true,
                            "node": "NRqCLTmhTLuSxzlWcTae3A",
                            "relocating_node": null,
                            "shard": 1,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "IzFuExmARziQWcX8RlaZdg"
                            }
                        },
                        {
                            "state": "STARTED",
                            "primary": false,
                            "node": "EizpW8QWRty_T1nJpr-dNQ",
                            "relocating_node": null,
                            "shard": 1,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "XqIv4y1rQf6aL5C63Xsbhg"
                            }
                        }
                    ],
                    "2": [
                        {
                            "state": "STARTED",
                            "primary": false,
                            "node": "q--ANfDnTKW2WS9pEBuLWQ",
                            "relocating_node": null,
                            "shard": 2,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "XCAywOULRf66CR2xugkIpg"
                            }
                        },
                        {
                            "state": "STARTED",
                            "primary": true,
                            "node": "EizpW8QWRty_T1nJpr-dNQ",
                            "relocating_node": null,
                            "shard": 2,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "yNuj-Rw7QkC74opnoRQIqQ"
                            }
                        }
                    ],
                    "3": [
                        {
                            "state": "STARTED",
                            "primary": true,
                            "node": "q--ANfDnTKW2WS9pEBuLWQ",
                            "relocating_node": null,
                            "shard": 3,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "foOkK0oWTAaFTg-M41sMgQ"
                            }
                        },
                        {
                            "state": "STARTED",
                            "primary": false,
                            "node": "NRqCLTmhTLuSxzlWcTae3A",
                            "relocating_node": null,
                            "shard": 3,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "MdjjvB9KTfu4gs_skXDyXg"
                            }
                        }
                    ],
                    "4": [
                        {
                            "state": "STARTED",
                            "primary": false,
                            "node": "q--ANfDnTKW2WS9pEBuLWQ",
                            "relocating_node": null,
                            "shard": 4,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "exBumbxRT6KY7LVmGOSIZA"
                            }
                        },
                        {
                            "state": "STARTED",
                            "primary": true,
                            "node": "NRqCLTmhTLuSxzlWcTae3A",
                            "relocating_node": null,
                            "shard": 4,
                            "index": "shakespeare",
                            "allocation_id": {
                                "id": "pUhEb1k5TC24EKD-OjS7Iw"
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

func newPod(name, namespace string) corev1.Pod {
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				label.VersionLabelName: "7.1.0",
			},
		},
	}
	return p
}

func TestDirectCluster_FilterDeletablePods(t *testing.T) {
	var clusterState esclient.ClusterState
	b := []byte(ClusterStateSample)
	err := json.Unmarshal(b, &clusterState)
	if err != nil {
		t.Error(err)
	}
	pod1 := newPod("elasticsearch-sample-es-79gc6p57rs", "default")
	pod2 := newPod("elasticsearch-sample-es-fnsgkkdl85", "default")
	pod3 := newPod("elasticsearch-sample-es-jfpqbt2s4q", "default")
	pod4 := newPod("elasticsearch-sample-es-nope", "default")

	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "elasticsearch-sample",
		},
	}

	type fields struct {
		esClient      client.Client
		observerState observer.State
	}
	type args struct {
		k                  k8s.Client
		es                 v1alpha1.Elasticsearch
		podsState          mutation.PodsState
		performableChanges *mutation.PerformableChanges
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErr    bool
		assertions func(t *testing.T, performableChanges *mutation.PerformableChanges)
	}{
		{
			name: "Do not delete a pod with migrating data",
			args: args{
				es: es,
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod.PodWithConfig{Pod: pod1},
							pod.PodWithConfig{Pod: pod2},
						},
					},
				},
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1.Name: pod1,
						pod2.Name: pod2,
						pod3.Name: pod3,
					},
				},
			},
			fields: fields{
				observerState: observer.State{
					ClusterState: &clusterState,
				},
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToDelete, 0)
			},
		},
		{
			name: "Delete a pod with no data",
			args: args{
				es: es,
				performableChanges: &mutation.PerformableChanges{
					Changes: mutation.Changes{
						ToDelete: pod.PodsWithConfig{
							pod.PodWithConfig{Pod: pod1},
							pod.PodWithConfig{Pod: pod2},
							pod.PodWithConfig{Pod: pod4},
						},
					},
				},
				podsState: mutation.PodsState{
					RunningReady: map[string]corev1.Pod{
						pod1.Name: pod1,
						pod2.Name: pod2,
						pod3.Name: pod3,
					},
				},
			},
			fields: fields{
				observerState: observer.State{
					ClusterState: &clusterState,
				},
			},
			assertions: func(t *testing.T, performableChanges *mutation.PerformableChanges) {
				assert.Len(t, performableChanges.ToDelete, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &DirectCluster{
				esClient:      tt.fields.esClient,
				observerState: tt.fields.observerState,
			}
			if err := c.FilterDeletablePods(tt.args.k, tt.args.es, tt.args.podsState, tt.args.performableChanges); (err != nil) != tt.wantErr {
				t.Errorf("DirectCluster.FilterDeletablePods() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
