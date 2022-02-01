// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shutdown

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

// NodeShutdown implements the shutdown.Interface with the Elasticsearch node shutdown API. It is not safe to call methods
// on this struct concurrently from multiple go-routines.
type NodeShutdown struct {
	c           esclient.Client
	typ         esclient.ShutdownType
	reason      string
	podToNodeID map[string]string
	shutdowns   map[string]esclient.NodeShutdown
	once        sync.Once
	log         logr.Logger
}

var _ Interface = &NodeShutdown{}

// NewNodeShutdown creates a new NodeShutdown struct restricted to one type of shutdown (typ); podToNodeID is mapping from
// K8s Pod name to Elasticsearch node ID; reason is an arbitrary bit of metadata that will be attached to each node shutdown
// request in Elasticsearch and can help to track and audit shutdown requests.
func NewNodeShutdown(c esclient.Client, podToNodeID map[string]string, typ esclient.ShutdownType, reason string, l logr.Logger) *NodeShutdown {
	return &NodeShutdown{
		c:           c,
		typ:         typ,
		podToNodeID: podToNodeID,
		reason:      reason,
		log:         l,
	}
}

func (ns *NodeShutdown) initOnce(ctx context.Context) error {
	var err error
	ns.once.Do(func() {
		var r esclient.ShutdownResponse
		r, err = ns.c.GetShutdown(ctx, nil)
		if err != nil {
			err = fmt.Errorf("while getting node shutdowns: %w", err)
			return
		}
		shutdowns := map[string]esclient.NodeShutdown{}
		for _, n := range r.Nodes {
			ns.log.V(1).Info("Existing shutdown", "type", n.Type, "node_id", n.NodeID, "status", n.Status)
			shutdowns[n.NodeID] = n
		}
		ns.shutdowns = shutdowns
	})
	return err
}

func (ns *NodeShutdown) lookupNodeID(podName string) (string, error) {
	nodeID, exists := ns.podToNodeID[podName]
	if !exists {
		return "", fmt.Errorf("node %s currently not member of the cluster", podName)
	}
	return nodeID, nil
}

// ReconcileShutdowns retrieves ongoing shutdowns and based on the given node names either cancels or creates new
// shutdowns.
func (ns *NodeShutdown) ReconcileShutdowns(ctx context.Context, leavingNodes []string) error {
	if err := ns.initOnce(ctx); err != nil {
		return err
	}
	// cancel all ongoing shutdowns for the current shutdown type
	if len(leavingNodes) == 0 {
		return ns.Clear(ctx, nil)
	}

	for _, node := range leavingNodes {
		nodeID, err := ns.lookupNodeID(node)
		if err != nil {
			return err
		}
		if shutdown, exists := ns.shutdowns[nodeID]; exists && shutdown.Is(ns.typ) {
			continue
		}
		ns.log.V(1).Info("Requesting shutdown", "type", ns.typ, "node", node, "node_id", nodeID)
		// in case of type=restart we are relying on the default allocation_delay of 5 min see
		// https://www.elastic.co/guide/en/elasticsearch/reference/7.15/put-shutdown.html
		if err := ns.c.PutShutdown(ctx, nodeID, ns.typ, ns.reason); err != nil {
			return fmt.Errorf("on put shutdown (type: %s) for node %s: %w", ns.typ, node, err)
		}
		// update the internal cache with the information about the new shutdown
		shutdown, err := ns.c.GetShutdown(ctx, &nodeID)
		if err != nil {
			return fmt.Errorf("on get shutdown (type; %s) for node %s: %w", ns.typ, node, err)
		}
		if len(shutdown.Nodes) != 1 {
			return fmt.Errorf("expected exactly one shutdown response got %d", len(shutdown.Nodes))
		}
		ns.shutdowns[nodeID] = shutdown.Nodes[0]
	}
	return nil
}

// ShutdownStatus returns the current shutdown status for the given node. It returns an error if no shutdown is in
// progress.
func (ns *NodeShutdown) ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error) {
	if err := ns.initOnce(ctx); err != nil {
		return NodeShutdownStatus{}, err
	}
	nodeID, err := ns.lookupNodeID(podName)
	if err != nil {
		return NodeShutdownStatus{}, err
	}
	shutdown, exists := ns.shutdowns[nodeID]
	if !exists {
		return NodeShutdownStatus{}, fmt.Errorf("no shutdown in progress for %s", podName)
	}
	logStatus(ns.log, podName, shutdown)
	return NodeShutdownStatus{
		Status:      shutdown.Status,
		Explanation: shutdown.ShardMigration.Explanation,
	}, nil
}

func logStatus(logger logr.Logger, podName string, shutdown esclient.NodeShutdown) {
	switch shutdown.Status {
	case esclient.ShutdownComplete:
		logger.Info("Node shutdown complete, can start node deletion", "type", shutdown.Type, "node", podName)
	case esclient.ShutdownInProgress:
		logger.V(1).Info("Node shutdown not over yet, hold off with node deletion", "type", shutdown.Type, "node", podName)
	case esclient.ShutdownStalled:
		logger.Info("Node shutdown stalled, user intervention maybe required if condition persists", "type", shutdown.Type, "explanation", shutdown.ShardMigration.Explanation, "node", podName)
	case esclient.ShutdownNotStarted:
		logger.Info("Unexpected: node shutdown could not be started", "type", shutdown.Type, "explanation", shutdown.ShardMigration.Explanation, "node", podName)
	}
}

// Clear deletes shutdown requests matching the type of the NodeShutdown field typ and the given optional status.
// Depending on the progress of the shutdown in question this means either a cancellation of the shutdown or a clean-up
// after shutdown completion.
func (ns *NodeShutdown) Clear(ctx context.Context, status *esclient.ShutdownStatus) error {
	if err := ns.initOnce(ctx); err != nil {
		return err
	}
	for _, s := range ns.shutdowns {
		if s.Is(ns.typ) && (status == nil || s.Status == *status) {
			ns.log.V(1).Info("Deleting shutdown", "type", ns.typ, "node_id", s.NodeID)
			if err := ns.c.DeleteShutdown(ctx, s.NodeID); err != nil {
				return fmt.Errorf("while deleting shutdown for %s: %w", s.NodeID, err)
			}
		}
	}
	return nil
}
