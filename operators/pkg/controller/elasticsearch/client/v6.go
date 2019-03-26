// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"path"

	"github.com/pkg/errors"
)

type clientV6 struct {
	baseClient
}

func (c *clientV6) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info
	return info, c.get(ctx, "/", &info)
}

func (c *clientV6) GetClusterState(ctx context.Context) (ClusterState, error) {
	var clusterState ClusterState
	return clusterState, c.get(ctx, "/_cluster/state/dispatcher,master_node,nodes,routing_table", &clusterState)
}

func (c *clientV6) UpdateSettings(ctx context.Context, settings Settings) error {
	return c.put(ctx, "/_cluster/settings", &settings, nil)
}

func (c *clientV6) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	allocationSetting := ClusterRoutingAllocation{AllocationSettings{ExcludeName: nodes, Enable: "all"}}
	return c.put(ctx, "/_cluster/settings", allocationSetting, nil)
}

func (c *clientV6) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health
	return result, c.get(ctx, "/_cluster/health", &result)
}

func (c *clientV6) GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error) {
	var result map[string]SnapshotRepository
	return result[name], c.get(ctx, path.Join("/_snapshot", name), &result)
}

func (c *clientV6) DeleteSnapshotRepository(ctx context.Context, name string) error {
	return c.delete(ctx, path.Join("/_snapshot", name), nil, nil)
}

func (c *clientV6) UpsertSnapshotRepository(ctx context.Context, name string, repository SnapshotRepository) error {
	return c.put(ctx, path.Join("/_snapshot", name), repository, nil)
}

func (c *clientV6) GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error) {
	var result SnapshotsList
	return result, c.get(ctx, path.Join("/_snapshot", repo, "_all"), &result)
}

func (c *clientV6) TakeSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.put(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

func (c *clientV6) DeleteSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.delete(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

func (c *clientV6) SetMinimumMasterNodes(ctx context.Context, n int) error {
	zenSettings := DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: n},
		Persistent: DiscoveryZen{MinimumMasterNodes: n},
	}
	return c.put(ctx, "/_cluster/settings", &zenSettings, nil)
}

func (c *clientV6) ReloadSecureSettings(ctx context.Context) error {
	return c.post(ctx, "/_nodes/reload_secure_settings", nil, nil)
}

func (c *clientV6) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes
	// restrict call to basic node info only
	return nodes, c.get(ctx, "/_nodes/_all/jvm,settings", &nodes)
}

func (c *clientV6) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	return license.License, c.get(ctx, "/_xpack/license", &license)
}

func (c *clientV6) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	return response, c.post(ctx, "/_xpack/license", licenses, &response)
}

func (c *clientV6) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	return errors.New("Not supported in Elasticsearch 6.x")
}

func (c *clientV6) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	return errors.New("Not supported in Elasticsearch 6.x")
}

// Equal returns true if c2 can be considered the same as c
func (c *clientV6) Equal(c2 Client) bool {
	other, ok := c2.(*clientV6)
	if !ok {
		return false
	}
	return c.baseClient.equal(&other.baseClient)
}

var _ Client = &clientV6{}
