// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

const (
	// DefaultVotingConfigExclusionsTimeout is the default timeout for setting voting exclusions.
	DefaultVotingConfigExclusionsTimeout = "30s"
	// DefaultReqTimeout is the default timeout used when performing HTTP calls against Elasticsearch
	DefaultReqTimeout = 3 * time.Minute
)

// BasicAuth contains credentials for an Elasticsearch user.
type BasicAuth struct {
	Name     string
	Password string
}

type IndexRole struct {
	Names      []string `json:"names,omitempty"`
	Privileges []string `json:",omitempty"`
}

// Role represents an Elasticsearch role.
type Role struct {
	Cluster []string    `json:"cluster,omitempty"`
	Indices []IndexRole `json:"indices,omitempty"`
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client interface {
	AllocationSetter
	ShardLister
	LicenseClient
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
	// ClusterBootstrappedForZen2 returns true if the cluster is relying on zen2 orchestration.
	ClusterBootstrappedForZen2(ctx context.Context) (bool, error)
	// UpdateRemoteClusterSettings updates the remote clusters of a cluster.
	UpdateRemoteClusterSettings(ctx context.Context, settings RemoteClustersSettings) error
	// AddVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// If timeout is the empty string, the default is used.
	//
	// Introduced in: Elasticsearch 7.0.0
	AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error
	// DeleteVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// Introduced in: Elasticsearch 7.0.0
	DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error
	// Request exposes a low level interface to the underlying HTTP client e.g. for testing purposes.
	// The Elasticsearch endpoint will be added automatically to the request URL which should therefore just be the path
	// with a leading /
	Request(ctx context.Context, r *http.Request) (*http.Response, error)
}

// NewElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(
	dialer net.Dialer,
	esURL string,
	esUser BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) Client {
	base := &baseClient{
		Endpoint: esURL,
		User:     esUser,
		caCerts:  caCerts,
		HTTP:     common.HTTPClient(dialer, caCerts),
	}
	return versioned(base, v)
}

// APIError is a non 2xx response from the Elasticsearch API
type APIError struct {
	response *http.Response
}

// Error() implements the error interface.
func (e *APIError) Error() string {
	defer e.response.Body.Close()
	reason := "unknown"
	// Elasticsearch has a detailed error message in the response body
	var errMsg ErrorResponse
	err := json.NewDecoder(e.response.Body).Decode(&errMsg)
	if err == nil {
		reason = errMsg.Error.Reason
	}
	return fmt.Sprintf("%s: %s", e.response.Status, reason)
}

// IsNotFound checks whether the error was an HTTP 404 error.
func IsNotFound(err error) bool {
	switch err := err.(type) {
	case *APIError:
		return err.response.StatusCode == http.StatusNotFound
	default:
		return false
	}
}

// IsConflict checks whether the error was an HTTP 409 error.
func IsConflict(err error) bool {
	switch err := err.(type) {
	case *APIError:
		return err.response.StatusCode == http.StatusConflict
	default:
		return false
	}
}

// IsForbidden checks whether the error was an HTTP 403 error.
func IsForbidden(err error) bool {
	switch err := err.(type) {
	case *APIError:
		return err.response.StatusCode == http.StatusForbidden
	default:
		return false
	}
}
