// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var desiredNodesMinVersion = version.MinFor(8, 3, 0)
var deprecatedNodeVersionReqBodyParamMinVersion = version.MinFor(8, 13, 0)

type DesiredNodesClient interface {
	IsDesiredNodesSupported() bool
	// GetLatestDesiredNodes returns the latest desired nodes.
	GetLatestDesiredNodes(ctx context.Context) (LatestDesiredNodes, error)
	// UpdateDesiredNodes updates the desired nodes API.
	UpdateDesiredNodes(ctx context.Context, historyID string, version int64, desiredNodes DesiredNodes) error
	// DeleteDesiredNodes deletes the desired nodes from the cluster state.
	DeleteDesiredNodes(ctx context.Context) error
}

type LatestDesiredNodes struct {
	HistoryID    string        `json:"history_id"`
	Version      int64         `json:"version"`
	DesiredNodes []DesiredNode `json:"nodes"`
}

type DesiredNodes struct {
	DesiredNodes []DesiredNode `json:"nodes"`
}

type DesiredNode struct {
	Settings        map[string]interface{} `json:"settings"`
	ProcessorsRange ProcessorsRange        `json:"processors_range"`
	Memory          string                 `json:"memory"`
	Storage         string                 `json:"storage"`
	NodeVersion     string                 `json:"node_version,omitempty"` // deprecated in 8.13+
}

type ProcessorsRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max,omitempty"`
}

func (c *baseClient) GetLatestDesiredNodes(_ context.Context) (LatestDesiredNodes, error) {
	return LatestDesiredNodes{}, c.desiredNodesNotAvailable()
}

func (c *baseClient) UpdateDesiredNodes(_ context.Context, _ string, _ int64, _ DesiredNodes) error {
	return c.desiredNodesNotAvailable()
}

func (c *baseClient) DeleteDesiredNodes(_ context.Context) error {
	return c.desiredNodesNotAvailable()
}

func (c *baseClient) desiredNodesNotAvailable() error {
	return fmt.Errorf("the desired nodes API is not available in Elasticsearch %s, it requires %s", c.version, desiredNodesMinVersion)
}

func (c *baseClient) IsDesiredNodesSupported() bool {
	return c.version.GTE(desiredNodesMinVersion)
}

func (c *clientV8) GetLatestDesiredNodes(ctx context.Context) (LatestDesiredNodes, error) {
	var latestDesiredNodes LatestDesiredNodes
	err := c.get(ctx, "/_internal/desired_nodes/_latest", &latestDesiredNodes)
	return latestDesiredNodes, err
}

func (c *clientV8) UpdateDesiredNodes(ctx context.Context, historyID string, version int64, desiredNodes DesiredNodes) error {
	// remove deprecated field depending on the version
	if c.version.GTE(deprecatedNodeVersionReqBodyParamMinVersion) {
		for i := range desiredNodes.DesiredNodes {
			desiredNodes.DesiredNodes[i].NodeVersion = ""
		}
	}
	return c.put(
		ctx,
		fmt.Sprintf("/_internal/desired_nodes/%s/%d", historyID, version),
		&desiredNodes, nil)
}

func (c *clientV8) DeleteDesiredNodes(ctx context.Context) error {
	return c.delete(ctx, "/_internal/desired_nodes")
}
