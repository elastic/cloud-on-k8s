// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	elasticsearch6 "github.com/elastic/go-elasticsearch/v6"
	esapi6 "github.com/elastic/go-elasticsearch/v6/esapi"
	elasticsearch7 "github.com/elastic/go-elasticsearch/v7"
	esapi7 "github.com/elastic/go-elasticsearch/v7/esapi"
	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	esapi8 "github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/elastic/go-elasticsearch/v8/esutil"
)

// putGenericClusterSettings is a generic helper method use to put cluster settings.
func (c *elasticsearchClients) putGenericClusterSettings(
	ctx context.Context,
	settings io.Reader,
	responseHandler func(response *Response) error,
) error {
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Cluster.PutSettings(
				settings,
				es.Cluster.PutSettings.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Cluster.PutSettings(
				settings,
				es.Cluster.PutSettings.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Cluster.PutSettings(
				settings,
				es.Cluster.PutSettings.WithContext(ctx),
			)
		},
		ResponseHandler: responseHandler,
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Cluster.Health(
				es.Cluster.Health.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Cluster.Health(
				es.Cluster.Health.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Cluster.Health(
				es.Cluster.Health.WithContext(ctx),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&result),
	}); err != nil {
		return result, err
	}

	return result, nil
}

func (c *elasticsearchClients) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes

	// restrict call to basic node info only
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Nodes.Info(
				es.Nodes.Info.WithContext(ctx),
				es.Nodes.Info.WithNodeID("_all"),
				es.Nodes.Info.WithMetric("jvm", "settings"),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Nodes.Info(
				es.Nodes.Info.WithContext(ctx),
				es.Nodes.Info.WithNodeID("_all"),
				es.Nodes.Info.WithMetric("jvm", "settings"),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Nodes.Info(
				es.Nodes.Info.WithContext(ctx),
				es.Nodes.Info.WithNodeID("_all"),
				es.Nodes.Info.WithMetric("jvm", "settings"),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&nodes),
	}); err != nil {
		return nodes, err
	}

	return nodes, nil
}

func (c *elasticsearchClients) GetNodesStats(ctx context.Context) (NodesStats, error) {
	var nodesStats NodesStats
	// restrict call to basic node info only
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Nodes.Stats(
				es.Nodes.Stats.WithContext(ctx),
				es.Nodes.Stats.WithNodeID("_all"),
				es.Nodes.Stats.WithMetric("os"),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Nodes.Stats(
				es.Nodes.Stats.WithContext(ctx),
				es.Nodes.Stats.WithNodeID("_all"),
				es.Nodes.Stats.WithMetric("os"),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Nodes.Stats(
				es.Nodes.Stats.WithContext(ctx),
				es.Nodes.Stats.WithNodeID("_all"),
				es.Nodes.Stats.WithMetric("os"),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&nodesStats),
	}); err != nil {
		return nodesStats, err
	}

	return nodesStats, nil
}

func (c *elasticsearchClients) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Info(
				es.Info.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Info(
				es.Info.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Info(
				es.Info.WithContext(ctx),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&info),
	}); err != nil {
		return info, err
	}

	return info, nil
}

func (c *elasticsearchClients) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.XPack.LicenseGet(
				es.XPack.LicenseGet.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.License.Get(
				es.License.Get.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.License.Get(
				es.License.Get.WithContext(ctx),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&license),
	}); err != nil {
		return license.License, err
	}

	return license.License, nil
}

func (c *elasticsearchClients) SetMinimumMasterNodes(ctx context.Context, minimumMasterNodes int) error {
	discoverySettings := esutil.NewJSONReader(DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: minimumMasterNodes},
		Persistent: DiscoveryZen{MinimumMasterNodes: minimumMasterNodes},
	})

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Cluster.PutSettings(
				discoverySettings,
				es.Cluster.PutSettings.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Cluster.PutSettings(
				discoverySettings,
				es.Cluster.PutSettings.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return nil, errors.New("setting minimum master nodes not supported in Elasticsearch 8.x")
		},
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) AddVotingConfigExclusions(ctx context.Context, excludeNodes []string) error {
	url := fmt.Sprintf(
		"/_cluster/voting_config_exclusions/%s?timeout=%s",
		strings.Join(excludeNodes, ","),
		DefaultVotingConfigExclusionsTimeout,
	)

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return nil, errors.New("voting config exclusions not supported in Elasticsearch 6.x")
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				return nil, err
			}
			return customRequest7(ctx, es, req)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			req, err := http.NewRequest(http.MethodPost, url, nil)
			if err != nil {
				return nil, err
			}
			return customRequest8(ctx, es, req)
		},
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) DeleteVotingConfigExclusions(ctx context.Context) error {
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return nil, errors.New("voting config exclusions not supported in Elasticsearch 6.x")
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			req, err := http.NewRequest(
				http.MethodDelete, "/_cluster/voting_config_exclusions?wait_for_removal=true", nil,
			)
			if err != nil {
				return nil, err
			}
			return customRequest7(ctx, es, req)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			req, err := http.NewRequest(
				http.MethodDelete, "/_cluster/voting_config_exclusions?wait_for_removal=true", nil,
			)
			if err != nil {
				return nil, err
			}
			return customRequest8(ctx, es, req)
		},
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) GetClusterRoutingAllocation(ctx context.Context) (ClusterRoutingAllocation, error) {
	var settings ClusterRoutingAllocation

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Cluster.GetSettings(
				es.Cluster.GetSettings.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Cluster.GetSettings(
				es.Cluster.GetSettings.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return nil, errors.New("setting minimum master nodes not supported in Elasticsearch 8.x")
		},
		ResponseHandler: esErrorOrDecodeJSON(&settings),
	}); err != nil {
		return settings, err
	}

	return settings, nil
}

func (c *elasticsearchClients) updateAllocationEnable(ctx context.Context, value string) error {
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

	return c.putGenericClusterSettings(ctx, esutil.NewJSONReader(allocationSettings), nil)
}

func (c *elasticsearchClients) EnableShardAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "all")
}

func (c *elasticsearchClients) DisableReplicaShardsAllocation(ctx context.Context) error {
	return c.updateAllocationEnable(ctx, "primaries")
}

func (c *elasticsearchClients) SyncedFlush(ctx context.Context) error {
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Indices.FlushSynced(
				es.Indices.FlushSynced.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Indices.FlushSynced(
				es.Indices.FlushSynced.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Indices.FlushSynced(
				es.Indices.FlushSynced.WithContext(ctx),
			)
		},
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) ReloadSecureSettings(ctx context.Context) error {
	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Nodes.ReloadSecureSettings(
				es.Nodes.ReloadSecureSettings.WithContext(ctx),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Nodes.ReloadSecureSettings(
				es.Nodes.ReloadSecureSettings.WithContext(ctx),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Nodes.ReloadSecureSettings(
				es.Nodes.ReloadSecureSettings.WithContext(ctx),
			)
		},
	}); err != nil {
		return err
	}

	return nil
}

func (c *elasticsearchClients) UpdateLicense(
	ctx context.Context,
	licenses LicenseUpdateRequest,
) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.XPack.LicensePost(
				es.XPack.LicensePost.WithContext(ctx),
				es.XPack.LicensePost.WithBody(esutil.NewJSONReader(licenses)),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.License.Post(
				es.License.Post.WithContext(ctx),
				es.License.Post.WithBody(esutil.NewJSONReader(licenses)),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.License.Post(
				es.License.Post.WithContext(ctx),
				es.License.Post.WithBody(esutil.NewJSONReader(licenses)),
			)
		},
		ResponseHandler: esErrorOrDecodeJSON(&response),
	}); err != nil {
		return response, err
	}

	return response, nil
}

func (c *elasticsearchClients) ExcludeFromShardAllocation(nodes string) error {
	// TODO: get context as a parameter
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()

	allocationSettings := ClusterRoutingAllocation{
		Transient: AllocationSettings{
			Cluster: ClusterRoutingSettings{
				Routing: RoutingSettings{
					Allocation: RoutingAllocationSettings{
						Exclude: AllocationExclude{
							Name: nodes,
						},
					},
				},
			},
		},
	}
	return c.putGenericClusterSettings(ctx, esutil.NewJSONReader(allocationSettings), nil)
}

func (c *elasticsearchClients) GetShards() (Shards, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReqTimeout)
	defer cancel()

	var shards Shards

	if err := c.doVersionedRequestWithUnversionedResponse(versionedRequestWithUnversionedResponse{
		ES6: func(es *elasticsearch6.Client) (*esapi6.Response, error) {
			return es.Cat.Shards(
				es.Cat.Shards.WithContext(ctx),
				es.Cat.Shards.WithFormat("json"),
			)
		},
		ES7: func(es *elasticsearch7.Client) (*esapi7.Response, error) {
			return es.Cat.Shards(
				es.Cat.Shards.WithContext(ctx),
				es.Cat.Shards.WithFormat("json"),
			)
		},
		ES8: func(es *elasticsearch8.Client) (*esapi8.Response, error) {
			return es.Cat.Shards(
				es.Cat.Shards.WithContext(ctx),
				es.Cat.Shards.WithFormat("json"),
			)
		},
	}); err != nil {
		return shards, err
	}

	return shards, nil
}
