// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
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
	withShutdown set.StringSet
	calls        [][]string
}

func (f *fakeNodesWithShutdown) NodesWithShutdown(_ context.Context, podNames []string) (set.StringSet, error) {
	f.calls = append(f.calls, podNames)
	nodes := set.Make()
	for _, podName := range podNames {
		if f.withShutdown.Has(podName) {
			nodes.Add(podName)
		}
	}
	return nodes, nil
}

func Test_leavingNodes(t *testing.T) {
	tests := []struct {
		name    string
		desired set.StringSet
		// nodes are the nodes given to members and draining
		nodes []string
		// members are the members of the cluster, nil if Elasticsearch must not be requested
		members set.StringSet
		// withShutdown are the nodes with a shutdown registered, nil if Elasticsearch is not reachable
		withShutdown set.StringSet
		// reported are the shutdown statuses in the last downscale status, by node name
		reported         map[string]esclient.ShutdownStatus
		wantMembers      []string
		wantDraining     []string
		wantShutdownArgs [][]string
	}{
		{
			name:         "no given node is leaving: Elasticsearch is not requested",
			desired:      set.Make("data-2"),
			nodes:        []string{"data-0", "data-1"},
			withShutdown: set.Make("data-0"),
		},
		{
			name:             "only the given nodes that are leaving are members or draining",
			desired:          set.Make("data-1", "data-2", "data-3"),
			nodes:            []string{"data-0", "data-1", "data-2"},
			members:          set.Make("data-0", "data-1", "data-3"),
			withShutdown:     set.Make("data-0", "data-2", "data-3"),
			wantMembers:      []string{"data-1"},
			wantDraining:     []string{"data-2"},
			wantShutdownArgs: [][]string{{"data-1", "data-2"}},
		},
		{
			name:    "Elasticsearch is not reachable: the nodes with a reported shutdown are draining",
			desired: set.Make("data-1", "data-2", "data-3"),
			nodes:   []string{"data-0", "data-1", "data-2"},
			members: set.Make("data-1"),
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
			require.ElementsMatch(t, tt.wantMembers, members.AsSlice())

			draining, err := leaving.draining(context.Background(), tt.nodes)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantDraining, draining.AsSlice())

			var shutdownArgs [][]string
			if fakeShutdown != nil {
				shutdownArgs = fakeShutdown.calls
			}
			require.Equal(t, tt.wantShutdownArgs, shutdownArgs)
		})
	}
}
