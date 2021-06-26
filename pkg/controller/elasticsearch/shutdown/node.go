// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package shutdown

import (
	"context"

	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
)

type NodeShutdown struct {
	c           esclient.Client
	podToNodeID map[string]string
}

var _ Interface = &NodeShutdown{}

func NewNodeShutdown(c esclient.Client, podToNodeID map[string]string) *NodeShutdown {
	return &NodeShutdown{
		c:           c,
		podToNodeID: podToNodeID,
	}
}

func (ns *NodeShutdown) RequestShutdown(ctx context.Context, leavingNodes []string) error {
	return nil
}

func (ns *NodeShutdown) ShutdownStatus(ctx context.Context, podName string) (ShutdownResponse, error) {
	return ShutdownResponse{}, nil
}
