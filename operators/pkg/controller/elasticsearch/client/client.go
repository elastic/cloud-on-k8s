// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"time"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("client")
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

// // Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client interface {
	Equal(other Client) bool
	// GetClusterInfo get the cluster information at /
	GetClusterInfo(ctx context.Context) (Info, error)
	// GetClusterState returns the current cluster state
	GetClusterState(ctx context.Context) (ClusterState, error)
	// ExcludeFromShardAllocation takes a comma-separated string of node names and
	// configures transient allocation excludes for the given nodes.
	ExcludeFromShardAllocation(ctx context.Context, nodes string) error
	// GetClusterHealth calls the _cluster/health api.
	GetClusterHealth(ctx context.Context) (Health, error)
	// GetSnapshotRepository retrieves the currently configured snapshot repository with the given name.
	GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error)
	// DeleteSnapshotRepository tries to delete the snapshot repository identified by name.
	DeleteSnapshotRepository(ctx context.Context, name string) error
	// UpsertSnapshotRepository inserts or updates the given snapshot repository
	UpsertSnapshotRepository(ctx context.Context, name string, repository SnapshotRepository) error
	// GetAllSnapshots returns a list of all snapshots for the given repository.
	GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error)
	// TakeSnapshot takes a new cluster snapshot with the given name into the given repository.
	TakeSnapshot(ctx context.Context, repo string, snapshot string) error
	// DeleteSnapshot deletes the given snapshot from the given repository.
	DeleteSnapshot(ctx context.Context, repo string, snapshot string) error
	// SetMinimumMasterNodes sets the transient and persistent setting of the same name in cluster settings.
	SetMinimumMasterNodes(ctx context.Context, n int) error
	// ReloadSecureSettings will decrypt and re-read the entire keystore, on every cluster node,
	// but only the reloadable secure settings will be applied
	ReloadSecureSettings(ctx context.Context) error
	// GetNodes calls the _nodes api to return a map(nodeName -> Node)
	GetNodes(ctx context.Context) (Nodes, error)
	// GetLicense returns the currently applied license. Can be empty.
	GetLicense(ctx context.Context) (License, error)
	// UpdateLicense attempts to update cluster license with the given licenses.
	UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error)
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
}
