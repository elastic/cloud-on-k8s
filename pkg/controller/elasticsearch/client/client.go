// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"net/http"
	"time"
)

const (
	// DefaultVotingConfigExclusionsTimeout is the default timeout for setting voting exclusions.
	DefaultVotingConfigExclusionsTimeout = "30s"
	// DefaultReqTimeout is the default timeout used when performing HTTP calls against Elasticsearch
	DefaultReqTimeout = 3 * time.Minute
)

// UserAuth is authentication information for the Elasticsearch client.
type UserAuth struct {
	Name     string
	Password string
}

// Role represents an Elasticsearch role.
type Role struct {
	Cluster []string `json:"cluster,omitempty"`
	/*Indices []struct {
		Names      []string `json:"names,omitempty"`
		Privileges []string `json:",omitempty"`
	} `json:"indices,omitempty"`
	Applications []struct {
		Application string   `json:"application"`
		Privileges  []string `json:"privileges"`
		Resources   []string `json:"resources,omitempty"`
	} `json:"applications,omitempty"`
	RunAs    []string `json:"run_as,omitempty"`
	Metadata *struct {
		Reserved bool `json:"_reserved"`
	} `json:"metadata,omitempty"`
	TransientMetadata *struct {
		Enabled bool `json:"enabled"`
	} `json:"transient_metadata,omitempty"`*/
}

// AllocationSetter captures Elasticsearch API calls around allocation filtering.
type AllocationSetter interface {
	// ExcludeFromShardAllocation takes a comma-separated string of node names and
	// configures transient allocation exclusions for the given nodes.
	ExcludeFromShardAllocation(nodes string) error
}

// ShardLister captures Elasticsearch API calls around shards retrieval.
type ShardLister interface {
	GetShards() (Shards, error)
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client interface {
	AllocationSetter
	ShardLister
	// Close idle connections in the underlying http client.
	Close()
	// Equal returns true if other can be considered as the same client.
	Equal(other Client) bool
	// GetClusterInfo get the cluster information at /
	GetClusterInfo(ctx context.Context) (Info, error)
	// GetClusterRoutingAllocation retrieves the cluster routing allocation settings.
	GetClusterRoutingAllocation(ctx context.Context) (ClusterRoutingAllocation, error)
	// DisableReplicaShardsAllocation disables shards allocation on the cluster (only primaries are allocated).
	DisableReplicaShardsAllocation(ctx context.Context) error
	// EnableShardAllocation enables shards allocation on the cluster.
	EnableShardAllocation(ctx context.Context) error
	// SyncedFlush requests a synced flush on the cluster.
	// This is "best-effort", see https://www.elastic.co/guide/en/elasticsearch/reference/current/indices-synced-flush.html.
	SyncedFlush(ctx context.Context) error
	// GetClusterHealth calls the _cluster/health api.
	GetClusterHealth(ctx context.Context) (Health, error)
	// SetMinimumMasterNodes sets the transient and persistent setting of the same name in cluster settings.
	SetMinimumMasterNodes(ctx context.Context, n int) error
	// ReloadSecureSettings will decrypt and re-read the entire keystore, on every cluster node,
	// but only the reloadable secure settings will be applied
	ReloadSecureSettings(ctx context.Context) error
	// GetNodes calls the _nodes api to return a map(nodeName -> Node)
	GetNodes(ctx context.Context) (Nodes, error)
	// GetNodesStats calls the _nodes/stats api to return a map(nodeName -> NodeStats)
	GetNodesStats(ctx context.Context) (NodesStats, error)
	// GetLicense returns the currently applied license. Can be empty.
	GetLicense(ctx context.Context) (License, error)
	// UpdateLicense attempts to update cluster license with the given licenses.
	UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error)
	// AddVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// If timeout is the empty string, the default is used.
	//
	// Introduced in: Elasticsearch 7.0.0
	AddVotingConfigExclusions(ctx context.Context, nodeNames []string) error
	// DeleteVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// Introduced in: Elasticsearch 7.0.0
	DeleteVotingConfigExclusions(ctx context.Context) error
	// Request exposes a low level interface to the underlying HTTP client e.g. for testing purposes.
	// The Elasticsearch endpoint will be added automatically to the request URL which should therefore just be the path
	// with a leading /
	Request(ctx context.Context, r *http.Request) (*http.Response, error)
}

// IsNotFound checks whether the error was an HTTP 404 error.
func IsNotFound(err error) bool {
	switch err := err.(type) {
	case *officialAPIError:
		return err.statusCode == http.StatusNotFound
	default:
		return false
	}
}

// IsConflict checks whether the error was an HTTP 409 error.
func IsConflict(err error) bool {
	switch err := err.(type) {
	case *officialAPIError:
		return err.statusCode == http.StatusConflict
	default:
		return false
	}
}
