// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"crypto/x509"
	"fmt"
	"math"
	"net/http"
	"time"

	"go.elastic.co/apm/module/apmelasticsearch/v2"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	commonhttp "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/http"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/net"
)

const (
	// ESClientTimeoutAnnotation is the name of the annotation used to set the Elasticsearch client timeout.
	ESClientTimeoutAnnotation = "eck.k8s.elastic.co/es-client-timeout"
)

// DefaultESClientTimeout is the default timeout value for Elasticsearch requests.
var DefaultESClientTimeout = 3 * time.Minute

// BasicAuth contains credentials for an Elasticsearch user.
type BasicAuth struct {
	Name     string
	Password string
}

type IndexRole struct {
	Names                  []string `json:"names,omitempty"`
	Privileges             []string `json:",omitempty"`
	AllowRestrictedIndices *bool    `json:"allow_restricted_indices,omitempty" yaml:"allow_restricted_indices,omitempty"`
}

type ApplicationRole struct {
	Application string   `json:"application,omitempty"`
	Privileges  []string `json:"privileges,omitempty"`
	Resources   []string `json:"resources,omitempty"`
}

// Role represents an Elasticsearch role.
type Role struct {
	Cluster      []string          `json:"cluster,omitempty"`
	Indices      []IndexRole       `json:"indices,omitempty"`
	Applications []ApplicationRole `json:"applications,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client interface {
	AllocationSetter
	AutoscalingClient
	DesiredNodesClient
	ShardLister
	LicenseClient
	SecurityClient
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
	// RemoveTransientAllocationSettings removes allocation filters and enablement settings.
	RemoveTransientAllocationSettings(ctx context.Context) error

	// SyncedFlush requests a synced flush on the cluster. Deprecated in 7.6, removed in 8.0.
	// This is "best-effort", see https://www.elastic.co/guide/en/elasticsearch/reference/current/indices-synced-flush.html.
	SyncedFlush(ctx context.Context) error
	// Flush requests a flush on the cluster.
	Flush(ctx context.Context) error
	// GetClusterHealth calls the _cluster/health api.
	GetClusterHealth(ctx context.Context) (Health, error)
	// GetClusterHealthWaitForAllEvents calls _cluster/health?wait_for_events=languid&timeout=0s
	GetClusterHealthWaitForAllEvents(ctx context.Context) (Health, error)
	// GetClusterState calls the _cluster/state api.
	GetClusterState(ctx context.Context) (ClusterState, error)
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
	// GetRemoteClusterSettings retrieves the remote clusters of a cluster.
	GetRemoteClusterSettings(ctx context.Context) (RemoteClustersSettings, error)
	// AddVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	// Introduced in: Elasticsearch 7.0.0
	AddVotingConfigExclusions(ctx context.Context, nodeNames []string) error
	// DeleteVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// Introduced in: Elasticsearch 7.0.0
	DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error
	// GetShutdown returns information about ongoing node shutdowns.
	// Introduced in: Elasticsearch 7.14.0
	GetShutdown(ctx context.Context, nodeID *string) (ShutdownResponse, error)
	// PutShutdown initiates a node shutdown procedure for the given node.
	// Introduced in: Elasticsearch 7.14.0
	PutShutdown(ctx context.Context, nodeID string, shutdownType ShutdownType, reason string) error
	// DeleteShutdown attempts to cancel an ongoing node shutdown.
	// Introduced in: Elasticsearch 7.14.0
	DeleteShutdown(ctx context.Context, nodeID string) error
	// Request exposes a low level interface to the underlying HTTP client e.g. for testing purposes.
	// The Elasticsearch endpoint will be added automatically to the request URL which should therefore just be the path
	// with a leading /
	Request(ctx context.Context, r *http.Request) (*http.Response, error)
	// Version returns the Elasticsearch version this client is constructed for which should equal the minimal version
	// in the cluster.
	Version() version.Version
	// HasProperties checks whether this client has the indicated properties.
	HasProperties(version version.Version, user BasicAuth, url URLProvider, caCerts []*x509.Certificate) bool
}

// Timeout returns the Elasticsearch client timeout value for the given Elasticsearch resource.
func Timeout(ctx context.Context, es esv1.Elasticsearch) time.Duration {
	return annotation.ExtractTimeout(ctx, es.ObjectMeta, ESClientTimeoutAnnotation, DefaultESClientTimeout)
}

func formatAsSeconds(d time.Duration) string {
	return fmt.Sprintf("%.0fs", math.Round(d.Seconds()))
}

// NewElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(
	dialer net.Dialer,
	es types.NamespacedName,
	esURL URLProvider,
	esUser BasicAuth,
	v version.Version,
	caCerts []*x509.Certificate,
	timeout time.Duration,
	debug bool,
) Client {
	client := commonhttp.Client(dialer, caCerts, timeout)
	client.Transport = apmelasticsearch.WrapRoundTripper(client.Transport)
	base := &baseClient{
		URLProvider: esURL,
		User:        esUser,
		caCerts:     caCerts,
		HTTP:        client,
		es:          es,
		debug:       debug,
	}
	return versioned(base, v)
}
