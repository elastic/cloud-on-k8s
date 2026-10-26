// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// unreachableESState is an ESState that fails to retrieve the nodes, to check that it is not requested.
type unreachableESState struct {
	ESState
}

func (unreachableESState) NodeNameToID() (map[string]string, error) {
	return nil, errors.New("unreachable")
}

// fakeNodesWithShutdown is a shutdown.Interface that returns the nodes with a shutdown among the given ones, and records
// the nodes it was given.
type fakeNodesWithShutdown struct {
	shutdown.Interface
	withShutdown sets.Set[string]
	calls        [][]string
}

func (f *fakeNodesWithShutdown) NodesWithShutdown(_ context.Context, podNames []string) (sets.Set[string], error) {
	f.calls = append(f.calls, sets.List(sets.New(podNames...)))
	nodes := sets.New[string]()
	for _, podName := range podNames {
		if f.withShutdown.Has(podName) {
			nodes.Insert(podName)
		}
	}
	return nodes, nil
}

func Test_leavingNodes(t *testing.T) {
	tests := []struct {
		name    string
		desired sets.Set[string]
		// nodes are the nodes given to members and draining
		nodes []string
		// members are the members of the cluster, nil if Elasticsearch must not be requested
		members sets.Set[string]
		// withShutdown are the nodes with a shutdown registered, nil if Elasticsearch is not reachable
		withShutdown sets.Set[string]
		// reported are the shutdown statuses in the last downscale status, by node name
		reported         map[string]esclient.ShutdownStatus
		wantMembers      []string
		wantDraining     []string
		wantShutdownArgs [][]string
	}{
		{
			name:         "no given node is leaving: Elasticsearch is not requested",
			desired:      sets.New("data-2"),
			nodes:        []string{"data-0", "data-1"},
			withShutdown: sets.New("data-0"),
		},
		{
			name:             "only the given nodes that are leaving are members or draining",
			desired:          sets.New("data-1", "data-2", "data-3"),
			nodes:            []string{"data-0", "data-1", "data-2"},
			members:          sets.New("data-0", "data-1", "data-3"),
			withShutdown:     sets.New("data-0", "data-2", "data-3"),
			wantMembers:      []string{"data-1"},
			wantDraining:     []string{"data-2"},
			wantShutdownArgs: [][]string{{"data-1", "data-2"}},
		},
		{
			name:    "Elasticsearch is not reachable: the nodes with a reported shutdown are draining",
			desired: sets.New("data-1", "data-2", "data-3"),
			nodes:   []string{"data-0", "data-1", "data-2"},
			members: sets.New("data-1"),
			reported: map[string]esclient.ShutdownStatus{
				"data-0": esclient.ShutdownInProgress,
				"data-1": esclient.ShutdownNotStarted,
				"data-2": esclient.ShutdownComplete,
				"data-3": esclient.ShutdownInProgress,
			},
			wantMembers:  []string{"data-1"},
			wantDraining: []string{"data-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var esState ESState = unreachableESState{}
			if tt.members != nil {
				esState = clusterMembers{members: tt.members}
			}
			var nodeShutdown shutdown.Interface
			var fakeShutdown *fakeNodesWithShutdown
			if tt.withShutdown != nil {
				fakeShutdown = &fakeNodesWithShutdown{withShutdown: tt.withShutdown}
				nodeShutdown = fakeShutdown
			}
			var lastDownscale esv1.DownscaleOperation
			for node, status := range tt.reported {
				lastDownscale.Nodes = append(lastDownscale.Nodes, esv1.DownscaledNode{Name: node, ShutdownStatus: string(status)})
			}
			leaving := newLeavingNodes(tt.desired, esState, nodeShutdown, lastDownscale)

			members, err := leaving.members(tt.nodes)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantMembers, members.UnsortedList())

			draining, err := leaving.draining(context.Background(), tt.nodes)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantDraining, draining.UnsortedList())

			var shutdownArgs [][]string
			if fakeShutdown != nil {
				shutdownArgs = fakeShutdown.calls
			}
			require.Equal(t, tt.wantShutdownArgs, shutdownArgs)
		})
	}
}

func Test_leavingNodes_unreached(t *testing.T) {
	tests := []struct {
		name    string
		desired sets.Set[string]
		// nodes are the nodes given to unreached
		nodes   []string
		members sets.Set[string]
		// partial is true if some nodes did not respond to the nodes request
		partial bool
		// withoutShutdown makes the shutdown interface unavailable
		withoutShutdown bool
		// partialErr makes the retrieval of the nodes response fail
		partialErr error
		wantErr    bool
		// reported are the shutdown statuses in the last downscale status, by node name
		reported map[string]esclient.ShutdownStatus
		want     []string
	}{
		{
			name:     "a leaving node missing from a partial response with a reported shutdown is unreached",
			desired:  sets.New("data-1", "data-2"),
			nodes:    []string{"data-0", "data-1", "data-2"},
			members:  sets.New("data-0"),
			partial:  true,
			reported: map[string]esclient.ShutdownStatus{"data-1": esclient.ShutdownInProgress, "data-2": esclient.ShutdownNotStarted},
			want:     []string{"data-1"},
		},
		{
			name:     "a leaving member is not unreached",
			desired:  sets.New("data-1"),
			nodes:    []string{"data-1"},
			members:  sets.New("data-1"),
			partial:  true,
			reported: map[string]esclient.ShutdownStatus{"data-1": esclient.ShutdownInProgress},
		},
		{
			name:     "a node that is not leaving is not unreached",
			desired:  sets.New("data-2"),
			nodes:    []string{"data-1"},
			members:  sets.New[string](),
			partial:  true,
			reported: map[string]esclient.ShutdownStatus{"data-1": esclient.ShutdownInProgress},
		},
		{
			name:     "no node is unreached if the response is complete",
			desired:  sets.New("data-1"),
			nodes:    []string{"data-1"},
			members:  sets.New[string](),
			reported: map[string]esclient.ShutdownStatus{"data-1": esclient.ShutdownInProgress},
		},
		{
			name:            "no node is unreached without the shutdown interface, as draining relies on the downscale status",
			desired:         sets.New("data-1"),
			nodes:           []string{"data-1"},
			members:         sets.New[string](),
			partial:         true,
			withoutShutdown: true,
			reported:        map[string]esclient.ShutdownStatus{"data-1": esclient.ShutdownInProgress},
		},
		{
			name:       "an error while retrieving the nodes response is returned",
			desired:    sets.New("data-1"),
			nodes:      []string{"data-1"},
			members:    sets.New[string](),
			partialErr: errors.New("boom"),
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodeShutdown shutdown.Interface
			if !tt.withoutShutdown {
				nodeShutdown = &fakeNodesWithShutdown{withShutdown: sets.New[string]()}
			}
			var lastDownscale esv1.DownscaleOperation
			for node, status := range tt.reported {
				lastDownscale.Nodes = append(lastDownscale.Nodes, esv1.DownscaledNode{Name: node, ShutdownStatus: string(status)})
			}
			leaving := newLeavingNodes(tt.desired, clusterMembers{members: tt.members, partial: tt.partial, partialErr: tt.partialErr}, nodeShutdown, lastDownscale)
			unreached, err := leaving.unreached(tt.nodes)
			require.Equal(t, tt.wantErr, err != nil, err)
			require.ElementsMatch(t, tt.want, unreached.UnsortedList())
		})
	}
}

// TestDesiredLeavingNodeNamesFromSpec checks that the leaving nodes derived from the spec are the ones derived from the
// StatefulSets that are built from it, which the downscale uses.
func TestDesiredLeavingNodeNamesFromSpec(t *testing.T) {
	const esName, namespace = "cluster", "ns"
	nodeSets := func(counts ...int32) []esv1.NodeSet {
		result := make([]esv1.NodeSet, 0, len(counts))
		for i, count := range counts {
			result = append(result, esv1.NodeSet{Name: string(rune('a' + i)), Count: count})
		}
		return result
	}
	tests := []struct {
		name string
		// actual and desired are the NodeSets of the actual StatefulSets, and of the spec
		actual, desired []esv1.NodeSet
		want            sets.Set[string]
	}{
		{
			name:    "nothing to remove",
			actual:  nodeSets(3, 2),
			desired: nodeSets(3, 2),
			want:    sets.New[string](),
		},
		{
			name:    "a NodeSet scales down",
			actual:  nodeSets(3, 2),
			desired: nodeSets(1, 2),
			want:    sets.New(esv1.StatefulSet(esName, "a")+"-1", esv1.StatefulSet(esName, "a")+"-2"),
		},
		{
			name:    "a NodeSet is removed",
			actual:  nodeSets(3, 2),
			desired: nodeSets(3),
			want:    sets.New(esv1.StatefulSet(esName, "b")+"-0", esv1.StatefulSet(esName, "b")+"-1"),
		},
		{
			name:    "a NodeSet scales up",
			actual:  nodeSets(1),
			desired: nodeSets(3),
			want:    sets.New[string](),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			scripts := &corev1.ConfigMap{}
			scripts.Namespace, scripts.Name = namespace, esv1.ScriptsConfigMap(esName)
			c := k8s.NewFakeClient(scripts)
			build := func(es esv1.Elasticsearch) es_sset.StatefulSetList {
				resolved, err := ResolveConfig(ctx, c, es, corev1.IPv4Protocol, false)
				require.NoError(t, err)
				resources, err := nodespec.BuildExpectedResources(ctx, c, es, nil, nil, false, metadata.Metadata{}, resolved)
				require.NoError(t, err)
				return resources.StatefulSets()
			}
			es := func(nodeSets []esv1.NodeSet) esv1.Elasticsearch {
				return esv1.Elasticsearch{
					Name: esName, Namespace: namespace,
					Spec: esv1.ElasticsearchSpec{Version: "8.17.0", NodeSets: nodeSets},
				}
			}
			actual := build(es(tt.actual))
			desired := es(tt.desired)
			require.Equal(t, tt.want, desiredLeavingNodeNames(ctx, build(desired), actual))
			require.Equal(t, tt.want, desiredLeavingNodeNamesFromSpec(ctx, desired, actual))
		})
	}
}
