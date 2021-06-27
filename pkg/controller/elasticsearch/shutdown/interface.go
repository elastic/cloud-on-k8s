// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package shutdown

import (
	"context"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

// TODO this is duplicating the API model in parts to bridge the gap between the old and the new world, maybe revisit

type NodeShutdownStatus struct {
	Status      esclient.ShutdownStatus
	Explanation string
}

type Interface interface {
	ReconcileShutdowns(ctx context.Context, leavingNodes []string) error
	ShutdownStatus(ctx context.Context, podName string) (NodeShutdownStatus, error)
}
