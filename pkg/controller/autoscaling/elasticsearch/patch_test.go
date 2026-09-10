// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// recordingPatchClient counts every patch issued and decodes the ops so tests can assert on
// both the number of round-trips and the exact paths/values that were sent.
type recordingPatchClient struct {
	k8s.Client
	patches int
	ops     []jsonPatchOp
}

func (c *recordingPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	raw, _ := patch.Data(obj)
	var decoded []jsonPatchOp
	_ = json.Unmarshal(raw, &decoded)
	c.ops = append(c.ops, decoded...)
	c.patches++
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// hasPath reports whether any recorded op targets exactly the given path.
func (c *recordingPatchClient) hasPath(path string) bool {
	for _, op := range c.ops {
		if op.Path == path {
			return true
		}
	}
	return false
}

// valueJSON returns the JSON-encoded value of the first op at path, or "" if absent.
func (c *recordingPatchClient) valueJSON(path string) string {
	for _, op := range c.ops {
		if op.Path == path {
			b, _ := json.Marshal(op.Value)
			return string(b)
		}
	}
	return ""
}

func esWithNodeSets(nodeSets ...esv1.NodeSet) *esv1.Elasticsearch {
	return &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "es", Namespace: "ns"},
		Spec:       esv1.ElasticsearchSpec{Version: "8.16.0", NodeSets: nodeSets},
	}
}

func liveES(t *testing.T, c k8s.Client) esv1.Elasticsearch {
	t.Helper()
	var live esv1.Elasticsearch
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "es"}, &live))
	return live
}

func TestPatchAutoscaledNodeSets(t *testing.T) {
	ns0 := "/spec/nodeSets/0" // base path for nodeSet at index 0

	tests := []struct {
		name        string
		current     *esv1.Elasticsearch
		stored      *esv1.Elasticsearch // nil → same as current (no concurrent write)
		reconciled  *esv1.Elasticsearch // nil → same as current (nothing changed)
		wantErr     bool
		wantPatches int
		verify      func(t *testing.T, c *recordingPatchClient, live esv1.Elasticsearch)
	}{
		{
			name: "writes count and resources for the changed nodeSet only",
			current: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 1},
				esv1.NodeSet{Name: "master", Count: 3},
			),
			reconciled: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 2, Resources: esv1.NodeSetResources{Storage: new(resource.MustParse("4Gi"))}},
				esv1.NodeSet{Name: "master", Count: 3},
			),
			wantPatches: 1,
			verify: func(t *testing.T, _ *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(2), live.Spec.NodeSets[0].Count)
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Storage)
				assert.Equal(t, "4Gi", live.Spec.NodeSets[0].Resources.Storage.String())
				assert.Equal(t, int32(3), live.Spec.NodeSets[1].Count, "untouched nodeSet must keep its count")
			},
		},
		{
			name:        "no request when nothing changed",
			current:     esWithNodeSets(esv1.NodeSet{Name: "data", Count: 2}),
			wantPatches: 0,
		},
		{
			name: "a nodeSet that left autoscaling keeps its values",
			current: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 4, Resources: esv1.NodeSetResources{Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2"))}}}},
				esv1.NodeSet{Name: "ml", Count: 1},
			),
			reconciled: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 4, Resources: esv1.NodeSetResources{Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2"))}}}},
				esv1.NodeSet{Name: "ml", Count: 2},
			),
			wantPatches: 1,
			verify: func(t *testing.T, _ *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(4), live.Spec.NodeSets[0].Count, "count must survive when the nodeSet leaves autoscaling")
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Requests.CPU)
				assert.Equal(t, "2", live.Spec.NodeSets[0].Resources.Requests.CPU.String())
				assert.Equal(t, int32(2), live.Spec.NodeSets[1].Count)
			},
		},
		{
			name:    "resources-only change: patch emitted without touching count",
			current: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 3}),
			reconciled: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 3, Resources: esv1.NodeSetResources{Storage: new(resource.MustParse("8Gi"))}},
			),
			wantPatches: 1,
			verify: func(t *testing.T, _ *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(3), live.Spec.NodeSets[0].Count)
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Storage)
				assert.Equal(t, "8Gi", live.Spec.NodeSets[0].Resources.Storage.String())
			},
		},
		{
			name:    "reconciled has more nodeSets than current: error",
			current: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1}),
			reconciled: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 2},
				esv1.NodeSet{Name: "extra", Count: 1},
			),
			wantErr:     true,
			wantPatches: 0,
		},
		{
			name: "reconciled has fewer nodeSets than current: extra current entries are not patched",
			current: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 1},
				esv1.NodeSet{Name: "master", Count: 3},
			),
			reconciled: esWithNodeSets(
				esv1.NodeSet{Name: "data", Count: 2},
			),
			wantPatches: 1,
			verify: func(t *testing.T, _ *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(2), live.Spec.NodeSets[0].Count, "data count must be updated")
				assert.Equal(t, int32(3), live.Spec.NodeSets[1].Count, "master must be left untouched")
			},
		},
		{
			name:        "fails rather than writing to the wrong nodeSet when names drift",
			current:     esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1}),
			stored:      esWithNodeSets(esv1.NodeSet{Name: "renamed", Count: 1}),
			reconciled:  esWithNodeSets(esv1.NodeSet{Name: "data", Count: 9}),
			wantErr:     true,
			wantPatches: 1,
			verify: func(t *testing.T, _ *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(1), live.Spec.NodeSets[0].Count)
			},
		},
		{
			name:    "first run: /resources absent, autoscaler sets memory+storage",
			current: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{Memory: new(resource.MustParse("4Gi"))}},
					Storage:   new(resource.MustParse("10Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources"), "must add /resources")
				v := c.valueJSON(ns0 + "/resources")
				assert.Contains(t, v, `"memory"`, "value must include memory")
				assert.Contains(t, v, `"storage"`, "value must include storage")
				assert.NotContains(t, v, `"cpu"`, "value must not include cpu")
				assert.False(t, c.hasPath(ns0+"/resources/requests"), "must not emit separate /requests op")
				assert.False(t, c.hasPath(ns0+"/resources/storage"), "must not emit separate /storage op")
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Requests.Memory)
				assert.Equal(t, "4Gi", live.Spec.NodeSets[0].Resources.Requests.Memory.String())
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Storage)
				assert.Equal(t, "10Gi", live.Spec.NodeSets[0].Resources.Storage.String())
			},
		},
		{
			name:    "first run: /resources absent, autoscaler sets cpu+memory+limits+storage",
			current: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 2,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("8Gi"))},
					},
					Storage: new(resource.MustParse("10Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources"), "must add /resources")
				v := c.valueJSON(ns0 + "/resources")
				assert.Contains(t, v, `"cpu"`)
				assert.Contains(t, v, `"memory"`)
				assert.Contains(t, v, `"storage"`)
			},
		},

		{
			name: "memory first write: /requests created when only storage exists",
			current: esWithNodeSets(esv1.NodeSet{
				Name:      "data",
				Count:     1,
				Resources: esv1.NodeSetResources{Storage: new(resource.MustParse("10Gi"))},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{Memory: new(resource.MustParse("4Gi"))}},
					Storage:   new(resource.MustParse("10Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests"), "must create /requests parent")
				v := c.valueJSON(ns0 + "/resources/requests")
				assert.Contains(t, v, `"memory"`)
				assert.NotContains(t, v, `"cpu"`)
				assert.False(t, c.hasPath(ns0+"/resources/requests/memory"), "must not emit sub-leaf when parent is new")
				assert.False(t, c.hasPath(ns0+"/resources/storage"), "must not touch unchanged storage")
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Requests.Memory)
				assert.Equal(t, "4Gi", live.Spec.NodeSets[0].Resources.Requests.Memory.String())
			},
		},
		{
			name: "cpu scaled with limits ratio: requests.cpu and limits.cpu leaves emitted, memory untouched",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("8")), Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.True(t, c.hasPath(ns0+"/resources/limits/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.False(t, c.hasPath(ns0+"/resources/limits/memory"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
				assert.False(t, c.hasPath(ns0+"/resources/limits"))
			},
		},
		{
			name: "memory scaled with limits ratio: requests.memory and limits.memory leaves emitted, cpu untouched",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("8Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("16Gi"))},
					},
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.True(t, c.hasPath(ns0+"/resources/limits/memory"))
				assert.False(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/limits/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
				assert.False(t, c.hasPath(ns0+"/resources/limits"))
			},
		},
		{
			name: "memory added with limits ratio: user requests.cpu not claimed",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2"))}},
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.True(t, c.hasPath(ns0+"/resources/limits"))
				v := c.valueJSON(ns0 + "/resources/limits")
				assert.Contains(t, v, `"memory"`)
				assert.NotContains(t, v, `"cpu"`)
				assert.False(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Requests.CPU)
				assert.Equal(t, "2", live.Spec.NodeSets[0].Resources.Requests.CPU.String())
			},
		},
		{
			name: "cpu scaled, user limits.memory in same limits group: limits.memory not claimed",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2"))},
						Limits:   commonv1.ResourceAllocations{Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("4"))},
						Limits:   commonv1.ResourceAllocations{Memory: new(resource.MustParse("8Gi"))},
					},
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/limits/memory"))
				assert.False(t, c.hasPath(ns0+"/resources/limits"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
			},
		},
		{
			name: "storage scaled: user requests.cpu and requests.memory not claimed",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("4Gi")),
					}},
					Storage: new(resource.MustParse("10Gi")),
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("4Gi")),
					}},
					Storage: new(resource.MustParse("20Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, live esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/storage"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
				assert.False(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.False(t, c.hasPath(ns0+"/resources"))
				require.NotNil(t, live.Spec.NodeSets[0].Resources.Storage)
				assert.Equal(t, "20Gi", live.Spec.NodeSets[0].Resources.Storage.String())
			},
		},
		{
			name: "memory and storage scaled: user requests.cpu not claimed",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("4Gi")),
					}},
					Storage: new(resource.MustParse("10Gi")),
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 1,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{
						CPU:    new(resource.MustParse("2")),
						Memory: new(resource.MustParse("8Gi")),
					}},
					Storage: new(resource.MustParse("20Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.True(t, c.hasPath(ns0+"/resources/storage"))
				assert.False(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
			},
		},
		{
			name: "all fields scaled: individual leaf ops emitted, no group-level ops",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 2,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("2")), Memory: new(resource.MustParse("4Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("8Gi"))},
					},
					Storage: new(resource.MustParse("10Gi")),
				},
			}),
			reconciled: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 4,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{
						Requests: commonv1.ResourceAllocations{CPU: new(resource.MustParse("4")), Memory: new(resource.MustParse("8Gi"))},
						Limits:   commonv1.ResourceAllocations{CPU: new(resource.MustParse("8")), Memory: new(resource.MustParse("16Gi"))},
					},
					Storage: new(resource.MustParse("20Gi")),
				},
			}),
			wantPatches: 1,
			verify: func(t *testing.T, c *recordingPatchClient, _ esv1.Elasticsearch) {
				t.Helper()
				assert.True(t, c.hasPath(ns0+"/resources/requests/cpu"))
				assert.True(t, c.hasPath(ns0+"/resources/requests/memory"))
				assert.True(t, c.hasPath(ns0+"/resources/limits/cpu"))
				assert.True(t, c.hasPath(ns0+"/resources/limits/memory"))
				assert.True(t, c.hasPath(ns0+"/resources/storage"))
				assert.False(t, c.hasPath(ns0+"/resources"))
				assert.False(t, c.hasPath(ns0+"/resources/requests"))
				assert.False(t, c.hasPath(ns0+"/resources/limits"))
			},
		},
		{
			name: "no change: no patch issued",
			current: esWithNodeSets(esv1.NodeSet{
				Name:  "data",
				Count: 2,
				Resources: esv1.NodeSetResources{
					Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{Memory: new(resource.MustParse("4Gi"))}},
					Storage:   new(resource.MustParse("10Gi")),
				},
			}),
			wantPatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := tt.stored
			if stored == nil {
				stored = tt.current.DeepCopy()
			}
			reconciled := tt.reconciled
			if reconciled == nil {
				reconciled = tt.current.DeepCopy()
			}
			c := &recordingPatchClient{Client: k8s.NewFakeClient(stored)}

			err := patchAutoscaledNodeSets(context.Background(), c, tt.current, reconciled)
			if tt.wantErr {
				require.Error(t, err, "the test operation on the nodeSet name must reject the patch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPatches, c.patches)
			if tt.verify != nil {
				tt.verify(t, c, liveES(t, c))
			}
		})
	}
}
