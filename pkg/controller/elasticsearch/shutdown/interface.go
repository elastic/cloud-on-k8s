// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shutdown

import (
	"context"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

var MinVersion = version.MinFor(7, 15, 2)

// NodeShutdownStatus describes the current shutdown status of an Elasticsearch node/Pod.
// Partially duplicates the Elasticsearch API to allow a version agnostic implementation in the controller.
type NodeShutdownStatus struct {
	Status      esclient.ShutdownStatus
	Explanation string
}

// Interface defines methods that both legacy shard migration based shutdown and new API based shutdowns implement to
// prepare node shutdowns.
type Interface interface {
	// ReconcileShutdowns retrieves ongoing shutdowns and creates the missing ones for the given nodes to shut down, which
	// can include nodes kept draining while their removal is held. If no node is given, it cancels the ongoing shutdowns
	// of the non-terminating nodes.
	ReconcileShutdowns(ctx context.Context, nodesToShutdown []string, terminatingNodes []string) error
	// ShutdownStatus returns the current shutdown status for the given node. It returns an error if no shutdown is in
	// progress.
	ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error)
	// NodesWithShutdown returns the given nodes that are members of the cluster and have a shutdown of the type managed
	// by this implementation registered in Elasticsearch.
	NodesWithShutdown(ctx context.Context, podNames []string) (set.StringSet, error)
}

type Observer interface {
	OnReconcileShutdowns(leavingNodes []string)
	OnShutdownStatus(podName string, status NodeShutdownStatus)
}

func WithObserver(implementation Interface, observer Observer) Interface {
	return &observed{
		Interface: implementation,
		observer:  observer,
	}
}

type observed struct {
	Interface
	observer Observer
}

func (o *observed) ReconcileShutdowns(ctx context.Context, nodesToShutdown []string, terminatingNodes []string) error {
	if o.observer != nil {
		o.observer.OnReconcileShutdowns(nodesToShutdown)
	}
	return o.Interface.ReconcileShutdowns(ctx, nodesToShutdown, terminatingNodes)
}

func (o *observed) ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error) {
	nodeShutdownStatus, err := o.Interface.ShutdownStatus(ctx, podName)
	if err == nil && o.observer != nil {
		o.observer.OnShutdownStatus(podName, nodeShutdownStatus)
	}
	return nodeShutdownStatus, err
}

var _ Interface = (*observed)(nil)
