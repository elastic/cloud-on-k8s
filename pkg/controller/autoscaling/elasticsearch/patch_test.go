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
	t.Run("writes count and resources for the changed nodeSet only", func(t *testing.T) {
		current := esWithNodeSets(
			esv1.NodeSet{Name: "data", Count: 1},
			esv1.NodeSet{Name: "master", Count: 3},
		)
		c := &countingPatchClient{Client: k8s.NewFakeClient(current.DeepCopy())}

		storage := resource.MustParse("4Gi")
		reconciled := current.DeepCopy()
		reconciled.Spec.NodeSets[0].Count = 2
		reconciled.Spec.NodeSets[0].Resources = esv1.NodeSetResources{Storage: &storage}

		require.NoError(t, patchAutoscaledNodeSets(context.Background(), c, current, reconciled))
		assert.Equal(t, 1, c.patches)

		live := liveES(t, c)
		assert.Equal(t, int32(2), live.Spec.NodeSets[0].Count)
		require.NotNil(t, live.Spec.NodeSets[0].Resources.Storage)
		assert.Equal(t, "4Gi", live.Spec.NodeSets[0].Resources.Storage.String())
		assert.Equal(t, int32(3), live.Spec.NodeSets[1].Count, "untouched nodeSet must keep its count")
	})

	t.Run("no request when nothing changed", func(t *testing.T) {
		current := esWithNodeSets(esv1.NodeSet{Name: "data", Count: 2})
		c := &countingPatchClient{Client: k8s.NewFakeClient(current.DeepCopy())}

		require.NoError(t, patchAutoscaledNodeSets(context.Background(), c, current, current.DeepCopy()))
		assert.Zero(t, c.patches, "an unchanged reconcile must not talk to the API server")
	})

	// The reason this is a JSON patch and not a Server-Side Apply. Apply prunes: a manager that
	// stops sending a field it owns deletes it. If a policy is removed from the autoscaler, or its
	// roles change so a nodeSet no longer matches, the recommendation no longer mentions that
	// nodeSet, and an Apply would drop its count. Absent count decodes to zero, which would scale
	// the tier down to nothing.
	t.Run("a nodeSet that left autoscaling keeps its values", func(t *testing.T) {
		cpu := resource.MustParse("2")
		managed := esv1.NodeSet{
			Name:      "data",
			Count:     4,
			Resources: esv1.NodeSetResources{Resources: commonv1.Resources{Requests: commonv1.ResourceAllocations{CPU: &cpu}}},
		}
		current := esWithNodeSets(managed, esv1.NodeSet{Name: "ml", Count: 1})
		c := &countingPatchClient{Client: k8s.NewFakeClient(current.DeepCopy())}

		// The autoscaler now only manages "ml"; "data" is untouched by the recommendation.
		reconciled := current.DeepCopy()
		reconciled.Spec.NodeSets[1].Count = 2

		require.NoError(t, patchAutoscaledNodeSets(context.Background(), c, current, reconciled))

		live := liveES(t, c)
		assert.Equal(t, int32(4), live.Spec.NodeSets[0].Count, "count must survive when the nodeSet leaves autoscaling")
		require.NotNil(t, live.Spec.NodeSets[0].Resources.Requests.CPU)
		assert.Equal(t, "2", live.Spec.NodeSets[0].Resources.Requests.CPU.String())
		assert.Equal(t, int32(2), live.Spec.NodeSets[1].Count)
	})

	t.Run("fails rather than writing to the wrong nodeSet when names drift", func(t *testing.T) {
		current := esWithNodeSets(esv1.NodeSet{Name: "data", Count: 1})
		// The stored object has a different nodeSet at index 0 than the caller believes.
		stored := esWithNodeSets(esv1.NodeSet{Name: "renamed", Count: 1})
		c := &countingPatchClient{Client: k8s.NewFakeClient(stored)}

		reconciled := current.DeepCopy()
		reconciled.Spec.NodeSets[0].Count = 9

		err := patchAutoscaledNodeSets(context.Background(), c, current, reconciled)
		require.Error(t, err, "the test operation on the nodeSet name must reject the patch")
		assert.Equal(t, int32(1), liveES(t, c).Spec.NodeSets[0].Count)
	})
}
