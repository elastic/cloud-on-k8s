// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// leavingNodes are the nodes the downscale wants to remove in a reconciliation, and the subsets of them that each phase
// relies on:
//   - members: the leaving nodes that are members of the cluster. The rolling upgrade leaves them to the downscale, the
//     other ones may need the upgrade to join the cluster, for the downscale to remove them.
//   - draining: the leaving nodes that have a remove-type shutdown registered. The downscale keeps their shutdowns, and
//     the forced upgrade does not delete them, as it would disrupt the shutdown that is migrating their shards away.
//
// The subsets are only retrieved for the given nodes, so that no request is made when none of them is leaving.
type leavingNodes struct {
	// desired are the Pods the downscale wants to remove, regardless of the downscale budget and invariants.
	desired set.StringSet
	// esState is used to retrieve the members of the cluster.
	esState ESState
	// nodeShutdown is used to retrieve the draining nodes. It is nil if Elasticsearch is not reachable or if it cannot be
	// created, in which case lastDownscale is used.
	nodeShutdown shutdown.Interface
	// lastDownscale is the downscale status reported while Elasticsearch was reachable.
	lastDownscale esv1.DownscaleOperation
}

// newLeavingNodes returns the leavingNodes of the given desired leaving Pods, see leavingNodes.
func newLeavingNodes(
	desired set.StringSet,
	esState ESState,
	nodeShutdown shutdown.Interface,
	lastDownscale esv1.DownscaleOperation,
) *leavingNodes {
	return &leavingNodes{desired: desired, esState: esState, nodeShutdown: nodeShutdown, lastDownscale: lastDownscale}
}

// names returns the names of the desired leaving Pods, sorted.
func (l *leavingNodes) names() []string {
	return l.desired.AsSortedSlice()
}

// among returns the given nodes that are leaving.
func (l *leavingNodes) among(nodes []string) []string {
	var leaving []string
	for _, node := range nodes {
		if l.desired.Has(node) {
			leaving = append(leaving, node)
		}
	}
	return leaving
}

// members returns the given nodes that are leaving and members of the cluster. Elasticsearch is not requested if none of
// them is leaving.
func (l *leavingNodes) members(nodes []string) (set.StringSet, error) {
	return membersAmong(l.esState, l.among(nodes))
}

// draining returns the given nodes that are leaving and have a remove-type shutdown registered. Elasticsearch is not
// requested if none of them is leaving. Without nodeShutdown, Elasticsearch is not reachable, which does not mean that
// the shutdowns stopped: the nodes with a shutdown in lastDownscale are draining.
func (l *leavingNodes) draining(ctx context.Context, nodes []string) (set.StringSet, error) {
	leaving := l.among(nodes)
	if len(leaving) == 0 {
		return set.Make(), nil
	}
	if l.nodeShutdown == nil {
		return nodesWithReportedShutdown(l.lastDownscale, leaving), nil
	}
	return l.nodeShutdown.NodesWithShutdown(ctx, leaving)
}

// desiredLeavingNodeNames returns the names of all the Pods that the expected StatefulSets no longer include, without
// applying any budget or invariant filter: this is the whole set of Pods the downscale wants to remove, not the ones it
// is allowed to remove now.
func desiredLeavingNodeNames(ctx context.Context, expectedStatefulSets es_sset.StatefulSetList, actualStatefulSets es_sset.StatefulSetList) set.StringSet {
	downscales, _, _ := calculateDownscales(ctx, downscaleState{}, expectedStatefulSets, actualStatefulSets, noDownscaleFilter)
	return set.Make(leavingNodeNames(downscales)...)
}

// nodesWithReportedShutdown returns the given nodes that have a shutdown in the given downscale status. The nodes the
// downscale has not requested a shutdown for are reported as not started. A node is reported in progress before its
// shutdown is requested, so a node whose request failed, for example because it never joined the cluster, is included.
func nodesWithReportedShutdown(downscale esv1.DownscaleOperation, nodes []string) set.StringSet {
	reported := set.Make()
	for _, node := range downscale.Nodes {
		if node.ShutdownStatus != string(esclient.ShutdownNotStarted) {
			reported.Add(node.Name)
		}
	}
	withShutdown := set.Make()
	for _, node := range nodes {
		if reported.Has(node) {
			withShutdown.Add(node)
		}
	}
	return withShutdown
}
