// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shutdown

import (
	"context"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

// NodeShutdownStatus describes the current shutdown status of an Elasticsearch node/Pod.
// Partially duplicates the Elasticsearch API to allow a version agnostic implementation in the controller.
type NodeShutdownStatus struct {
	Status      esclient.ShutdownStatus
	Explanation string
}

// Interface defines methods that both legacy shard migration based shutdown and new API based shutdowns implement to
// prepare node shutdowns.
type Interface interface {
	// ReconcileShutdowns retrieves ongoing shutdowns and based on the given node names either cancels or creates new
	// shutdowns.
	ReconcileShutdowns(ctx context.Context, leavingNodes []string) error
	// ShutdownStatus returns the current shutdown status for the given node. It returns an error if no shutdown is in
	// progress.
	ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error)
}
