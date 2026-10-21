// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
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
