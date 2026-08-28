// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"context"
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

// countingPatchClient records how many patches were issued, so the tests can assert that an
// unchanged reconcile talks to the API server not at all.
type countingPatchClient struct {
	k8s.Client
	patches int
}

func (c *countingPatchClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.patches++
	return c.Client.Patch(ctx, obj, patch, opts...)
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
	tests := []struct {
		name        string
		current     *esv1.Elasticsearch
		stored      *esv1.Elasticsearch // nil → same as current (no concurrent write)
		reconciled  *esv1.Elasticsearch // nil → same as current (nothing changed)
		wantErr     bool
		wantPatches int
		verify      func(t *testing.T, live esv1.Elasticsearch)
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
			verify: func(t *testing.T, live esv1.Elasticsearch) {
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
			// JSON patch does not prune: a nodeSet that leaves the autoscaling policy keeps
			// its last-written values. Server-Side Apply would drop the count on the next
			// reconcile that does not mention the nodeSet, scaling the tier down to zero.
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
			verify: func(t *testing.T, live esv1.Elasticsearch) {
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
			verify: func(t *testing.T, live esv1.Elasticsearch) {
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
			verify: func(t *testing.T, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(2), live.Spec.NodeSets[0].Count, "data count must be updated")
				assert.Equal(t, int32(3), live.Spec.NodeSets[1].Count, "master must be left untouched")
			},
		},
		{
			name:    "fails rather than writing to the wrong nodeSet when names drift",
			current: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1}),
			// The stored object has a different nodeSet at index 0 than the caller believes.
			stored:     esWithNodeSets(esv1.NodeSet{Name: "renamed", Count: 1}),
			reconciled: esWithNodeSets(esv1.NodeSet{Name: "data", Count: 9}),
			wantErr:    true,
			// The patch is sent to the API server (patches = 1) but rejected there by the test op;
			// patchAutoscaledNodeSets does not do a pre-flight name check in Go.
			wantPatches: 1,
			verify: func(t *testing.T, live esv1.Elasticsearch) {
				t.Helper()
				assert.Equal(t, int32(1), live.Spec.NodeSets[0].Count)
			},
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
			c := &countingPatchClient{Client: k8s.NewFakeClient(stored)}

			err := patchAutoscaledNodeSets(context.Background(), c, tt.current, reconciled)
			if tt.wantErr {
				require.Error(t, err, "the test operation on the nodeSet name must reject the patch")
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantPatches, c.patches)
			if tt.verify != nil {
				tt.verify(t, liveES(t, c))
			}
		})
	}
}
