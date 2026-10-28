// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"

	"k8s.io/apimachinery/pkg/util/sets"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/shutdown"
	es_sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
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
	desired sets.Set[string]
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
	desired sets.Set[string],
	esState ESState,
	nodeShutdown shutdown.Interface,
	lastDownscale esv1.DownscaleOperation,
) *leavingNodes {
	return &leavingNodes{desired: desired, esState: esState, nodeShutdown: nodeShutdown, lastDownscale: lastDownscale}
}

// names returns the names of the desired leaving Pods, sorted.
func (l *leavingNodes) names() []string {
	return sets.List(l.desired)
}

// members returns the given nodes that are leaving and members of the cluster. Elasticsearch is not requested if none of
// them is leaving.
func (l *leavingNodes) members(nodes []string) (sets.Set[string], error) {
	return membersAmong(l.esState, sets.New(nodes...).Intersection(l.desired).UnsortedList())
}

// draining returns the given nodes that are leaving and have a remove-type shutdown registered. Elasticsearch is not
// requested if none of them is leaving. Without nodeShutdown, because Elasticsearch is not reachable or the shutdown
// interface cannot be created, the shutdowns may still be running: the nodes with a shutdown in lastDownscale are draining.
func (l *leavingNodes) draining(ctx context.Context, nodes []string) (sets.Set[string], error) {
	leaving := sets.New(nodes...).Intersection(l.desired).UnsortedList()
	if len(leaving) == 0 {
		return sets.New[string](), nil
	}
	if l.nodeShutdown == nil {
		return nodesWithReportedShutdown(l.lastDownscale, leaving), nil
	}
	return l.nodeShutdown.NodesWithShutdown(ctx, leaving)
}

// unreached returns the given nodes that are leaving, missing from a partial nodes response, and have a shutdown in the
// last downscale status. Such a node may be a draining member of the cluster that did not respond: it has no node ID,
// so members and draining miss it, although its shutdown may still be running. The upgrades must leave it alone like
// the nodes they find draining. Nothing is returned if the nodes response is complete: a node missing from it is then
// not a member. Nothing is returned either without nodeShutdown, as draining then relies on the last downscale status
// for all the nodes. The protection is broader than needed, as the nodes that did not respond are not identified.
// A leaving node that left the cluster on its own, for example because its Pod restarted, is protected too while
// another node fails to answer: the upgrades that would repair it leave it alone until the response is complete or the
// node rejoins. Telling them apart would require the IDs of the nodes that did not respond.
func (l *leavingNodes) unreached(nodes []string) (sets.Set[string], error) {
	leaving := sets.New(nodes...).Intersection(l.desired)
	if leaving.Len() == 0 || l.nodeShutdown == nil {
		return sets.New[string](), nil
	}
	partial, err := l.esState.NodesResponsePartial()
	if err != nil || !partial {
		return sets.New[string](), err
	}
	members, err := membersAmong(l.esState, leaving.UnsortedList())
	if err != nil {
		return nil, err
	}
	return nodesWithReportedShutdown(l.lastDownscale, leaving.Difference(members).UnsortedList()), nil
}

// reportedStatus returns the shutdown status of the given node in the last downscale status.
func (l *leavingNodes) reportedStatus(node string) shutdown.NodeShutdownStatus {
	for _, reported := range l.lastDownscale.Nodes {
		if reported.Name != node {
			continue
		}
		status := shutdown.NodeShutdownStatus{Status: esclient.ShutdownStatus(reported.ShutdownStatus)}
		if reported.Explanation != nil {
			status.Explanation = *reported.Explanation
		}
		return status
	}
	return shutdown.NodeShutdownStatus{Status: esclient.ShutdownNotStarted}
}

// withUnreached returns the given leaving nodes, which an upgrade leaves alone, plus the unreached ones among the given
// nodes, see unreached. They are not part of draining, because a node without node ID cannot be given to the shutdown
// interface: its shutdown could not be requested nor kept. Every upgrade path that leaves the leaving nodes alone must
// go through here, for them to protect the unreached ones as well.
func (l *leavingNodes) withUnreached(leaving sets.Set[string], nodes []string) (sets.Set[string], error) {
	unreached, err := l.unreached(nodes)
	if err != nil {
		return nil, err
	}
	return unreached.Union(leaving), nil
}

// desiredLeavingNodeNames returns the names of all the Pods that the expected StatefulSets no longer include, without
// applying any budget or invariant filter: this is the whole set of Pods the downscale wants to remove, not the ones it
// is allowed to remove now.
func desiredLeavingNodeNames(ctx context.Context, expectedStatefulSets es_sset.StatefulSetList, actualStatefulSets es_sset.StatefulSetList) sets.Set[string] {
	downscales, _, _ := calculateDownscales(ctx, downscaleState{}, expectedStatefulSets, actualStatefulSets, noDownscaleFilter)
	return sets.New(leavingNodeNames(downscales)...)
}

// nodesWithReportedShutdown returns the given nodes that have a shutdown in the given downscale status. The nodes the
// downscale has not requested a shutdown for are reported as not started. A node is only reported in progress once its
// shutdown is confirmed, so a node whose request failed, for example because it never joined the cluster, is not included.
// A status written by an older operator version may still report unconfirmed nodes in progress, until the first
// reconciliation that reaches Elasticsearch rewrites it.
func nodesWithReportedShutdown(downscale esv1.DownscaleOperation, nodes []string) sets.Set[string] {
	reported := sets.New[string]()
	for _, node := range downscale.Nodes {
		if node.ShutdownStatus != string(esclient.ShutdownNotStarted) {
			reported.Insert(node.Name)
		}
	}
	withShutdown := sets.New[string]()
	for _, node := range nodes {
		if reported.Has(node) {
			withShutdown.Insert(node)
		}
	}
	return withShutdown
}
