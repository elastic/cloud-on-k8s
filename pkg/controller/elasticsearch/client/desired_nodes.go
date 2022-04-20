// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

var desiredNodesMinVersion = version.MustParse("8.1.0")

type DesiredNodesClient interface {
	IsDesiredNodesSupported() bool
	// UpdateDesiredNodes updates the desired nodes API.
	UpdateDesiredNodes(ctx context.Context, historyID string, version int64, desiredNodes DesiredNodes) error
	// DeleteDesiredNodes deletes the desired nodes from the cluster state.
	DeleteDesiredNodes(ctx context.Context) error
}

type DesiredNodes struct {
	DesiredNodes []DesiredNode `json:"nodes"`
}

type DesiredNode struct {
	Settings    map[string]interface{} `json:"settings"`
	Processors  int                    `json:"processors"`
	Memory      string                 `json:"memory"`
	Storage     string                 `json:"storage"`
	NodeVersion string                 `json:"node_version"`
}

func (c *baseClient) UpdateDesiredNodes(_ context.Context, _ string, _ int64, _ DesiredNodes) error {
	return c.desiredNodesNotAvailable()
}

func (c *baseClient) DeleteDesiredNodes(_ context.Context) error {
	return c.desiredNodesNotAvailable()
}

func (c *baseClient) desiredNodesNotAvailable() error {
	return fmt.Errorf("the desired node API is not available in Elasticsearch %s, it requires %s", c.version, desiredNodesMinVersion)
}

func (c *baseClient) IsDesiredNodesSupported() bool {
	return c.version.GTE(desiredNodesMinVersion)
}

func (c *clientV8) UpdateDesiredNodes(ctx context.Context, historyID string, version int64, desiredNodes DesiredNodes) error {
	return c.put(
		ctx,
		fmt.Sprintf("/_internal/desired_nodes/%s/%d", historyID, version),
		&desiredNodes, nil)
}

func (c *clientV8) DeleteDesiredNodes(ctx context.Context) error {
	return c.delete(ctx, "/_internal/desired_nodes")
}
