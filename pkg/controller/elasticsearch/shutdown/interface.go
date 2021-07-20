// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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

type Interface interface {
	ReconcileShutdowns(ctx context.Context, leavingNodes []string) error
	ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error)
}
