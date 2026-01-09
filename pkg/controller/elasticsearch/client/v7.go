// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/stringsutil"
)

var errNotSupportedInEs7x = errors.New("not supported in Elasticsearch 7.x")

type clientV7 struct {
	baseClient
}

func (c *clientV7) Version() version.Version {
	return c.version
}

func (c *clientV7) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info
	err := c.get(ctx, "/", &info)
	return info, err
}

func (c *clientV7) GetClusterRoutingAllocation(ctx context.Context) (ClusterRoutingAllocation, error) {
	var settings ClusterRoutingAllocation
	err := c.get(ctx, "/_cluster/settings", &settings)
	return settings, err
}

func (c *clientV7) updateAllocationEnable(ctx context.Context, value string) error {
	allocationSettings := ClusterRoutingAllocation{
		Transient: AllocationSettings{
			Cluster: ClusterRoutingSettings{
				Routing: RoutingSettings{
					Allocation: RoutingAllocationSettings{
						Enable: value,
					},
				},
			},
		},
	}
	return c.put(ctx, "/_cluster/settings", allocationSettings, nil)
}

func (c *clientV7) EnableShardAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "all")
}

func (c *clientV7) DisableReplicaShardsAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "primaries")
}

func (c *clientV7) RemoveTransientAllocationSettings(ctx context.Context) error {
	allocationSettings := struct {
		Transient struct {
			Exclude *string `json:"cluster.routing.allocation.exclude._name"`
			Enable  *string `json:"cluster.routing.allocation.enable"`
		} `json:"transient"`
	}{}
	return c.put(ctx, "/_cluster/settings", allocationSettings, nil)
}

func (c *clientV7) SyncedFlush(ctx context.Context) error {
	return c.post(ctx, "/_flush/synced", nil, nil)
}

func (c *clientV7) Flush(ctx context.Context) error {
	return c.post(ctx, "/_flush", nil, nil)
}

func (c *clientV7) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health
	err := c.get(ctx, "/_cluster/health", &result)
	return result, err
}

func (c *clientV7) GetClusterHealthWaitForAllEvents(ctx context.Context) (Health, error) {
	var result Health
	// wait for all events means wait for all events down to `languid` events which is the lowest event priority
	pathWithQuery := "/_cluster/health?wait_for_events=languid&timeout=0s"
	// ignore timeout errors as they are communicated in the returned payload and a timeout is to be expected
	// given the query parameters. 408 for other reasons than the clients timeout parameter should not happen
	// as they are expected only on idle connections https://go-review.googlesource.com/c/go/+/179457/4/src/net/http/transport.go#1931
	err := c.request(ctx, http.MethodGet, pathWithQuery, nil, &result, IsTimeout)
	return result, err
}

func (c *clientV7) ReloadSecureSettings(ctx context.Context) error {
	return c.post(ctx, "/_nodes/reload_secure_settings", nil, nil)
}

func (c *clientV7) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes
	// restrict call to minimal node information with a non-existent metric filter
	err := c.get(ctx, "/_nodes/_all/no-metrics", &nodes)
	return nodes, err
}

func (c *clientV7) GetNodesStats(ctx context.Context) (NodesStats, error) {
	var nodesStats NodesStats
	// restrict call to basic node info only
	err := c.get(ctx, "/_nodes/_all/stats/os", &nodesStats)
	return nodesStats, err
}

func (c *clientV7) UpdateRemoteClusterSettings(ctx context.Context, settings RemoteClustersSettings) error {
	return c.put(ctx, "/_cluster/settings", &settings, nil)
}

func (c *clientV7) GetRemoteClusterSettings(ctx context.Context) (RemoteClustersSettings, error) {
	remoteClustersSettings := RemoteClustersSettings{}
	err := c.get(ctx, "/_cluster/settings", &remoteClustersSettings)
	return remoteClustersSettings, err
}

func (c *clientV7) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	err := c.get(ctx, "/_license", &license)
	return license.License, err
}

func (c *clientV7) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	err := c.post(ctx, "/_license?acknowledge=true", licenses, &response)
	return response, err
}

func (c *clientV7) StartTrial(ctx context.Context) (StartTrialResponse, error) {
	var response StartTrialResponse
	err := c.post(ctx, "/_license/start_trial?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV7) StartBasic(ctx context.Context) (StartBasicResponse, error) {
	var response StartBasicResponse
	err := c.post(ctx, "/_license/start_basic?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV7) AddVotingConfigExclusions(ctx context.Context, nodeNames []string) error {
	var path string
	if c.version.GTE(version.From(7, 8, 0)) {
		path = fmt.Sprintf("/_cluster/voting_config_exclusions?node_names=%s", strings.Join(nodeNames, ","))
	} else {
		// versions < 7.8.0 or unversioned clients which is OK as this deprecated API will be supported until 8.0
		path = fmt.Sprintf("/_cluster/voting_config_exclusions/%s", strings.Join(nodeNames, ","))
	}

	if err := c.post(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to add to voting_config_exclusions")
	}
	return nil
}

func (c *clientV7) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions?wait_for_removal=%s",
		strconv.FormatBool(waitForRemoval),
	)

	if err := c.delete(ctx, path); err != nil {
		return errors.Wrap(err, "unable to delete /_cluster/voting_config_exclusions")
	}
	return nil
}

func (c *clientV7) GetShutdown(ctx context.Context, nodeID *string) (ShutdownResponse, error) {
	var r ShutdownResponse
	path := "/_nodes/shutdown"
	if nodeID != nil {
		path = fmt.Sprintf("/_nodes/%s/shutdown", *nodeID)
	}
	err := c.get(ctx, path, &r)
	return r, err
}

func (c *clientV7) PutShutdown(ctx context.Context, nodeID string, shutdownType ShutdownType, reason string) error {
	request := ShutdownRequest{
		Type:   shutdownType,
		Reason: reason,
	}
	return c.put(ctx, fmt.Sprintf("/_nodes/%s/shutdown", nodeID), request, nil)
}

func (c *clientV7) DeleteShutdown(ctx context.Context, nodeID string) error {
	return c.delete(ctx, fmt.Sprintf("/_nodes/%s/shutdown", nodeID))
}

func (c *clientV7) ClusterBootstrappedForZen2(ctx context.Context) (bool, error) {
	// Look at the current master node of the cluster: if it's running version 7.x.x or above,
	// the cluster has been bootstrapped.
	var response Nodes
	if err := c.get(ctx, "/_nodes/_master", &response); err != nil {
		return false, err
	}
	if len(response.Nodes) == 0 {
		// no known master node (yet), consider the cluster is not bootstrapped
		return false, nil
	}
	for _, master := range response.Nodes {
		return master.isV7OrAbove()
	}
	// should never happen since we ensured a single entry in the above map
	return false, errors.New("no master found in ClusterBootstrappedForZen2")
}

func (c *clientV7) GetClusterState(_ context.Context) (ClusterState, error) {
	return ClusterState{}, errors.New("cluster state is not supported in Elasticsearch 7.x")
}

func (c *clientV7) InvalidateCrossClusterAPIKey(context.Context, string) error {
	return errNotSupportedInEs7x
}

func (c *clientV7) CreateCrossClusterAPIKey(_ context.Context, _ CrossClusterAPIKeyCreateRequest) (CrossClusterAPIKeyCreateResponse, error) {
	return CrossClusterAPIKeyCreateResponse{}, errNotSupportedInEs7x
}

func (c *clientV7) UpdateCrossClusterAPIKey(_ context.Context, _ string, _ CrossClusterAPIKeyUpdateRequest) (CrossClusterAPIKeyUpdateResponse, error) {
	return CrossClusterAPIKeyUpdateResponse{}, errNotSupportedInEs7x
}

func (c *clientV7) GetCrossClusterAPIKeys(_ context.Context, _ string) (CrossClusterAPIKeyList, error) {
	return CrossClusterAPIKeyList{}, errNotSupportedInEs7x
}

func (c *clientV7) Request(ctx context.Context, r *http.Request) (*http.Response, error) {
	baseURL, err := c.URLProvider.URL()
	if err != nil {
		return nil, err
	}
	newURL, err := url.Parse(stringsutil.Concat(baseURL, r.URL.String()))
	if err != nil {
		return nil, err
	}
	r.URL = newURL
	return c.doRequest(ctx, r)
}

// Equal returns true if c2 can be considered the same as c
func (c *clientV7) Equal(c2 Client) bool {
	other, ok := c2.(*clientV7)
	if !ok {
		return false
	}
	return c.baseClient.equal(&other.baseClient)
}

var _ Client = &clientV7{}
