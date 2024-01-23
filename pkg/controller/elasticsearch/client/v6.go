// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/stringsutil"
)

var errNotSupportedInEs6x = errors.New("not supported in Elasticsearch 6.x")

type clientV6 struct {
	baseClient
}

func (c *clientV6) Version() version.Version {
	return c.version
}

func (c *clientV6) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info
	err := c.get(ctx, "/", &info)
	return info, err
}

func (c *clientV6) GetClusterRoutingAllocation(ctx context.Context) (ClusterRoutingAllocation, error) {
	var settings ClusterRoutingAllocation
	err := c.get(ctx, "/_cluster/settings", &settings)
	return settings, err
}

func (c *clientV6) updateAllocationEnable(ctx context.Context, value string) error {
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

func (c *clientV6) EnableShardAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "all")
}

func (c *clientV6) DisableReplicaShardsAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "primaries")
}

func (c *clientV6) RemoveTransientAllocationSettings(ctx context.Context) error {
	allocationSettings := struct {
		Transient struct {
			Exclude *string `json:"cluster.routing.allocation.exclude._name"`
			Enable  *string `json:"cluster.routing.allocation.enable"`
		} `json:"transient"`
	}{}
	return c.put(ctx, "/_cluster/settings", allocationSettings, nil)
}

func (c *clientV6) SyncedFlush(ctx context.Context) error {
	return c.post(ctx, "/_flush/synced", nil, nil)
}

func (c *clientV6) Flush(ctx context.Context) error {
	return c.post(ctx, "/_flush", nil, nil)
}

func (c *clientV6) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health
	err := c.get(ctx, "/_cluster/health", &result)
	return result, err
}

func (c *clientV6) GetClusterHealthWaitForAllEvents(ctx context.Context) (Health, error) {
	var result Health
	// wait for all events means wait for all events down to `languid` events which is the lowest event priority
	pathWithQuery := "/_cluster/health?wait_for_events=languid&timeout=0s"
	// ignore timeout errors as they are communicated in the returned payload and a timeout is to be expected
	// given the query parameters. 408 for other reasons than the clients timeout parameter should not happen
	// as they are expected only on idle connections https://go-review.googlesource.com/c/go/+/179457/4/src/net/http/transport.go#1931
	err := c.request(ctx, http.MethodGet, pathWithQuery, nil, &result, IsTimeout)
	return result, err
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
	// restrict call to minimal node information with a non-existent metric filter
	err := c.get(ctx, "/_nodes/_all/no-metrics", &nodes)
	return nodes, err
}

func (c *clientV6) GetNodesStats(ctx context.Context) (NodesStats, error) {
	var nodesStats NodesStats
	// restrict call to basic node info only
	err := c.get(ctx, "/_nodes/_all/stats/os", &nodesStats)
	return nodesStats, err
}

func (c *clientV6) UpdateRemoteClusterSettings(ctx context.Context, settings RemoteClustersSettings) error {
	return c.put(ctx, "/_cluster/settings", &settings, nil)
}

func (c *clientV6) GetRemoteClusterSettings(ctx context.Context) (RemoteClustersSettings, error) {
	remoteClustersSettings := RemoteClustersSettings{}
	err := c.get(ctx, "/_cluster/settings", &remoteClustersSettings)
	return remoteClustersSettings, err
}

func (c *clientV6) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	err := c.get(ctx, "/_xpack/license", &license)
	return license.License, err
}

func (c *clientV6) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	err := c.post(ctx, "/_xpack/license?acknowledge=true", licenses, &response)
	return response, err
}

func (c *clientV6) StartTrial(ctx context.Context) (StartTrialResponse, error) {
	var response StartTrialResponse
	err := c.post(ctx, "/_xpack/license/start_trial?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV6) StartBasic(ctx context.Context) (StartBasicResponse, error) {
	var response StartBasicResponse
	err := c.post(ctx, "/_xpack/license/start_basic?acknowledge=true", nil, &response)
	return response, err
}

func (c *clientV6) AddVotingConfigExclusions(_ context.Context, _ []string) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) DeleteVotingConfigExclusions(_ context.Context, _ bool) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) DeleteAutoscalingPolicies(_ context.Context) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) CreateAutoscalingPolicy(_ context.Context, _ string, _ v1alpha1.AutoscalingPolicy) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) GetAutoscalingCapacity(_ context.Context) (AutoscalingCapacityResult, error) {
	return AutoscalingCapacityResult{}, errNotSupportedInEs6x
}

func (c *clientV6) UpdateMLNodesSettings(_ context.Context, _ int32, _ string) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) GetShutdown(context.Context, *string) (ShutdownResponse, error) {
	return ShutdownResponse{}, errNotSupportedInEs6x
}

func (c *clientV6) PutShutdown(context.Context, string, ShutdownType, string) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) DeleteShutdown(context.Context, string) error {
	return errNotSupportedInEs6x
}

func (c *clientV6) ClusterBootstrappedForZen2(ctx context.Context) (bool, error) {
	// Look at the current master node of the cluster: if it's running version 7.x.x or above,
	// the cluster has been bootstrapped.
	// Even though c is a clientV6, it may be targeting a mixed v6/v7 having a v7 master.
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

func (c *clientV6) GetClusterState(_ context.Context) (ClusterState, error) {
	return ClusterState{}, errors.New("cluster state is not supported in Elasticsearch 6.x")
}

func (c *clientV6) Request(ctx context.Context, r *http.Request) (*http.Response, error) {
	newURL, err := url.Parse(stringsutil.Concat(c.Endpoint, r.URL.String()))
	if err != nil {
		return nil, err
	}
	r.URL = newURL
	return c.doRequest(ctx, r)
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
