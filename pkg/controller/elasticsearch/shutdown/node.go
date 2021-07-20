// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package shutdown

import (
	"context"
	"fmt"
	"sync"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

var log = ulog.Log.WithName("node-shutdown")

type NodeShutdown struct {
	c           esclient.Client
	reason      string
	podToNodeID map[string]string
	shutdowns   map[string]esclient.NodeShutdown
	once        sync.Once
}

var _ Interface = &NodeShutdown{}

func NewNodeShutdown(c esclient.Client, podToNodeID map[string]string, reason string) *NodeShutdown {
	return &NodeShutdown{
		c:           c,
		podToNodeID: podToNodeID,
		reason:      reason,
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
		for _, ns := range r.Nodes {
			log.V(1).Info("existing shutdown", "node", ns.NodeID, "type", ns.Type, "status", ns.Status)
			shutdowns[ns.NodeID] = ns
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

func (ns *NodeShutdown) ReconcileShutdowns(ctx context.Context, leavingNodes []string) error {
	if err := ns.initOnce(ctx); err != nil {
		return err
	}
	// cancel all ongoing shutdowns
	if len(leavingNodes) == 0 {
		return ns.clear(ctx, nil)
	}

	for _, node := range leavingNodes {
		nodeID, err := ns.lookupNodeID(node)
		if err != nil {
			return err
		}
		if shutdown, exists := ns.shutdowns[nodeID]; exists && shutdown.Is(esclient.Remove) {
			continue
		}
		log.V(1).Info("Requesting shutdown", "node", node, "node-id", nodeID)
		if err := ns.c.PutShutdown(ctx, nodeID, esclient.Remove, ns.reason); err != nil {
			return fmt.Errorf("on put shutdown %w", err)
		}
		shutdown, err := ns.c.GetShutdown(ctx, &nodeID)
		if err != nil || len(shutdown.Nodes) != 1 {
			return err
		}
		ns.shutdowns[nodeID] = shutdown.Nodes[0]
	}
	return nil
}

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
	return NodeShutdownStatus{
		Status:      shutdown.Status,
		Explanation: shutdown.ShardMigration.Explanation,
	}, nil
}

func (ns *NodeShutdown) clear(ctx context.Context, status *esclient.ShutdownStatus) error {
	if err := ns.initOnce(ctx); err != nil {
		return err
	}
	for _, s := range ns.shutdowns {
		if s.Is(esclient.Remove) && (status == nil || s.Status == *status) {
			log.V(1).Info("deleting/cancelling shutdown", "node-id", s.NodeID)
			if err := ns.c.DeleteShutdown(ctx, s.NodeID); err != nil {
				return fmt.Errorf("while deleting shutdown for %s: %w", s.NodeID, err)
			}
		}
	}
	return nil
}
