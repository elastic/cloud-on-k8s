// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// Info represents the response from /
type Info struct {
	ClusterName string `json:"cluster_name"`
	ClusterUUID string `json:"cluster_uuid"`
	Version     struct {
		Number string `json:"number"`
	} `json:"version"`
}

// Health represents the response from _cluster/health
type Health struct {
	ClusterName                 string                   `json:"cluster_name"`
	Status                      esv1.ElasticsearchHealth `json:"status"`
	TimedOut                    bool                     `json:"timed_out"`
	NumberOfNodes               int                      `json:"number_of_nodes"`
	NumberOfDataNodes           int                      `json:"number_of_data_nodes"`
	ActivePrimaryShards         int                      `json:"active_primary_shards"`
	ActiveShards                int                      `json:"active_shards"`
	RelocatingShards            int                      `json:"relocating_shards"`
	InitializingShards          int                      `json:"initializing_shards"`
	UnassignedShards            int                      `json:"unassigned_shards"`
	DelayedUnassignedShards     int                      `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int                      `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int                      `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int                      `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float32                  `json:"active_shards_percent_as_number"`
}

type ShardState string

// These are possible shard states
const (
	STARTED      ShardState = "STARTED"
	INITIALIZING ShardState = "INITIALIZING"
	RELOCATING   ShardState = "RELOCATING"
	UNASSIGNED   ShardState = "UNASSIGNED"
)

type ShardType string

const (
	Primary ShardType = "p"
	Replica ShardType = "r"
)

// Nodes partially models the response from a request to /_nodes
type Nodes struct {
	Nodes map[string]Node `json:"nodes"`
}

func (n Nodes) Names() []string {
	names := make([]string, 0, len(n.Nodes))
	for _, node := range n.Nodes {
		names = append(names, node.Name)
	}
	return names
}

// Node partially models an Elasticsearch node retrieved from /_nodes
type Node struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Roles   []string `json:"roles"`
}

func (n Node) isV7OrAbove() (bool, error) {
	v, err := version.Parse(n.Version)
	if err != nil {
		return false, errors.Wrap(err, fmt.Sprintf("unable to parse node version %s", n.Version))
	}
	return v.Major >= 7, nil
}

// NodesStats partially models the response from a request to /_nodes/stats
type NodesStats struct {
	Nodes map[string]NodeStats `json:"nodes"`
}

// NodeStats partially models an Elasticsearch node retrieved from /_nodes/stats
type NodeStats struct {
	Name string `json:"name"`
	OS   struct {
		CGroup struct {
			Memory struct {
				LimitInBytes string `json:"limit_in_bytes"`
			} `json:"memory"`
			CPU struct {
				CFSPeriodMicros int `json:"cfs_period_micros"`
				CFSQuotaMicros  int `json:"cfs_quota_micros"`
			} `json:"cpu"`
		} `json:"cgroup"`
	} `json:"os"`
}

// ClusterStateNode represents an element in the `node` structure in
// Elasticsearch cluster state.
type ClusterStateNode struct {
	Name             string `json:"name"`
	EphemeralID      string `json:"ephemeral_id"`
	TransportAddress string `json:"transport_address"`
	Attributes       struct {
		MlMachineMemory string `json:"ml.machine_memory"`
		MlMaxOpenJobs   string `json:"ml.max_open_jobs"`
		XpackInstalled  string `json:"xpack.installed"`
		MlEnabled       string `json:"ml.enabled"`
	} `json:"attributes"`
}

// Shards contains the shards in the Elasticsearch cluster
type Shards []Shard

// Shard partially models Elasticsearch cluster shard.
type Shard struct {
	Index    string     `json:"index"`
	Shard    string     `json:"shard"`
	State    ShardState `json:"state"`
	NodeName string     `json:"node"`
	Type     ShardType  `json:"prirep"`
}

type RoutingTable struct {
	Indices map[string]Shards `json:"indices"`
}

// GetShardsByNode returns shards by node.
// The result is a map with the name of the nodes as keys and the list of shards on the nodes as values.
func (s Shards) GetShardsByNode() map[string]Shards {
	result := make(map[string]Shards)
	for _, shard := range s {
		// Unassigned shards are ignored
		if len(shard.NodeName) > 0 {
			result[shard.NodeName] = append(result[shard.NodeName], shard)
		}
	}
	return result
}

// Strip extra information from the nodeName field
// eg. "cluster-node-2 -> 10.56.2.33 8DqGuLtrSNyMfE2EfKNDgg" becomes "cluster-node-2"
// see https://github.com/elastic/cloud-on-k8s/issues/1796
func (s *Shards) UnmarshalJSON(data []byte) error {
	type Alias Shards
	aux := (*Alias)(s)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	for i, shard := range *aux {
		if idx := strings.IndexByte(shard.NodeName, ' '); idx >= 0 {
			(*s)[i].NodeName = (*s)[i].NodeName[:idx]
		}
	}
	return nil
}

// IsRelocating is true if the shard is relocating to another node.
func (s Shard) IsRelocating() bool {
	return s.State == RELOCATING
}

// IsStarted is true if the shard is started on its current node.
func (s Shard) IsStarted() bool {
	return s.State == STARTED
}

// IsInitializing is true if the shard is currently initializing on the node.
func (s Shard) IsInitializing() bool {
	return s.State == INITIALIZING
}

// IsReplica is true if the shard is a replica.
func (s Shard) IsReplica() bool {
	return s.Type == Replica
}

// IsPrimary is true if the shard is a primary shard.
func (s Shard) IsPrimary() bool {
	return s.Type == Primary
}

// Key is a composite key of index name and shard number that identifies all
// copies of a shard across nodes.
func (s Shard) Key() string {
	return stringsutil.Concat(s.Index, "/", s.Shard)
}

// AllocationSettings model a subset of the supported attributes for dynamic Elasticsearch cluster settings.
type AllocationSettings struct {
	Cluster ClusterRoutingSettings `json:"cluster,omitempty"`
} // TODO awareness settings

type ClusterRoutingSettings struct {
	Routing RoutingSettings `json:"routing,omitempty"`
}

type RoutingSettings struct {
	Allocation RoutingAllocationSettings `json:"allocation,omitempty"`
}

type RoutingAllocationSettings struct {
	Exclude AllocationExclude `json:"exclude,omitempty"`
	Enable  string            `json:"enable,omitempty"`
}

type AllocationExclude struct {
	Name string `json:"_name,omitempty"`
}

func (s AllocationSettings) IsShardsAllocationEnabled() bool {
	enable := s.Cluster.Routing.Allocation.Enable
	return enable == "" || enable == "all"
}

// ClusterRoutingAllocation models a subset of transient allocation settings for an Elasticsearch cluster.
type ClusterRoutingAllocation struct {
	Transient AllocationSettings `json:"transient,omitempty"`
}

// DiscoveryZen set minimum number of master eligible nodes that must be visible to form a cluster.
type DiscoveryZen struct {
	MinimumMasterNodes int `json:"discovery.zen.minimum_master_nodes"`
}

// DiscoveryZenSettings are cluster settings related to the zen discovery mechanism.
type DiscoveryZenSettings struct {
	Transient  DiscoveryZen `json:"transient"`
	Persistent DiscoveryZen `json:"persistent"`
}

// ErrorResponse is an Elasticsearch error response.
type ErrorResponse struct {
	Status int `json:"status"`
	Error  struct {
		CausedBy struct {
			Reason string `json:"reason"`
			Type   string `json:"type"`
		} `json:"caused_by"`
		Reason    string `json:"reason"`
		Type      string `json:"type"`
		RootCause []struct {
			Reason string `json:"reason"`
			Type   string `json:"type"`
		} `json:"root_cause"`
	} `json:"error"`
}

// ElasticsearchLicenseType the type of a license.
type ElasticsearchLicenseType string

// Supported ElasticsearchLicenseTypes.
const (
	ElasticsearchLicenseTypeBasic      ElasticsearchLicenseType = "basic"
	ElasticsearchLicenseTypeTrial      ElasticsearchLicenseType = "trial"
	ElasticsearchLicenseTypeGold       ElasticsearchLicenseType = "gold"
	ElasticsearchLicenseTypePlatinum   ElasticsearchLicenseType = "platinum"
	ElasticsearchLicenseTypeEnterprise ElasticsearchLicenseType = "enterprise"
)

// ElasticsearchLicenseTypeOrder license types mapped to ints in increasing order of feature sets for sorting purposes.
var ElasticsearchLicenseTypeOrder = map[ElasticsearchLicenseType]int{
	ElasticsearchLicenseTypeBasic:      1,
	ElasticsearchLicenseTypeTrial:      2,
	ElasticsearchLicenseTypeGold:       3,
	ElasticsearchLicenseTypePlatinum:   4,
	ElasticsearchLicenseTypeEnterprise: 5,
}

// License models the Elasticsearch license applied to a cluster. Signature will be empty on reads. IssueDate,  ExpiryTime and Status can be empty on writes.
type License struct {
	Status             string     `json:"status,omitempty"`
	UID                string     `json:"uid"`
	Type               string     `json:"type"`
	IssueDate          *time.Time `json:"issue_date,omitempty"`
	IssueDateInMillis  int64      `json:"issue_date_in_millis"`
	ExpiryDate         *time.Time `json:"expiry_date,omitempty"`
	ExpiryDateInMillis int64      `json:"expiry_date_in_millis"`
	MaxNodes           int        `json:"max_nodes,omitempty"`
	MaxResourceUnits   int        `json:"max_resource_units,omitempty"`
	IssuedTo           string     `json:"issued_to"`
	Issuer             string     `json:"issuer"`
	StartDateInMillis  int64      `json:"start_date_in_millis"`
	Signature          string     `json:"signature,omitempty"`
}

// StartTime is the date as of which this license is valid.
func (l License) StartTime() time.Time {
	return time.Unix(0, l.StartDateInMillis*int64(time.Millisecond))
}

// ExpiryTime is the date as of which the license is no longer valid.
func (l License) ExpiryTime() time.Time {
	return time.Unix(0, l.ExpiryDateInMillis*int64(time.Millisecond))
}

// IsValid returns true if the license is still valid at the given point in time.
func (l License) IsValid(instant time.Time) bool {
	return (l.StartTime().Equal(instant) || l.StartTime().Before(instant)) &&
		l.ExpiryTime().After(instant)
}

// IsSupported returns true if the current license type is supported by the given version of Elasticsearch.
func (l License) IsSupported(v *version.Version) bool {
	if l.Type == string(ElasticsearchLicenseTypeEnterprise) && !v.GTE(version.MustParse("7.8.1")) {
		return false
	}
	return true
}

// LicenseUpdateRequest is the request to apply a license to a cluster. Licenses must contain signature.
type LicenseUpdateRequest struct {
	Licenses []License `json:"licenses"`
}

// LicenseUpdateResponse is the response to a license update request.
type LicenseUpdateResponse struct {
	Acknowledged bool `json:"acknowledged"`
	// LicenseStatus can be one of 'valid', 'invalid', 'expired'
	LicenseStatus string `json:"license_status"`
}

func (lr LicenseUpdateResponse) IsSuccess() bool {
	return lr.Acknowledged && lr.LicenseStatus == "valid"
}

// StartTrialResponse is the response to the start trial API call.
type StartTrialResponse struct {
	Acknowledged    bool   `json:"acknowledged"`
	TrialWasStarted bool   `json:"trial_was_started"`
	ErrorMessage    string `json:"error_message"`
}

func (sr StartTrialResponse) IsSuccess() bool {
	return sr.Acknowledged && sr.TrialWasStarted
}

// LicenseResponse is the response to GET _xpack/license. Licenses won't contain signature.
type LicenseResponse struct {
	License License `json:"license"`
}

// StartBasicResponse is the response to the start trial API call.
type StartBasicResponse struct {
	Acknowledged    bool   `json:"acknowledged"`
	BasicWasStarted bool   `json:"basic_was_started"`
	ErrorMessage    string `json:"error_message"`
}

// RemoteClustersSettings is used to build a request to update remote clusters.
type RemoteClustersSettings struct {
	PersistentSettings *SettingsGroup `json:"persistent,omitempty"`
}

// SettingsGroup is a group of persistent settings.
type SettingsGroup struct {
	Cluster RemoteClusters `json:"cluster,omitempty"`
}

// RemoteClusters models the configuration of the remote clusters.
type RemoteClusters struct {
	RemoteClusters map[string]RemoteCluster `json:"remote,omitempty"`
}

// RemoteCluster is the set of seeds to use in a remote cluster setting.
type RemoteCluster struct {
	Seeds []string `json:"seeds"`
}

// Hit represents a single search hit.
type Hit struct {
	Index  string                 `json:"_index"`
	Type   string                 `json:"_type"`
	ID     string                 `json:"_id"`
	Score  float64                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}

// Hits are the collections of search hits.
type Hits struct {
	Total json.RawMessage // model when needed
	Hits  []Hit           `json:"hits"`
}

// SearchResults are the results returned from a _search.
type SearchResults struct {
	Took   int
	Hits   Hits                       `json:"hits"`
	Shards json.RawMessage            // model when needed
	Aggs   map[string]json.RawMessage // model when needed
}

// ShutdownType is the set of different shutdown operation types supported by Elasticsearch.
type ShutdownType string

var (

	// Restart indicates the intent to restart an Elasticsearch node.
	Restart ShutdownType = "restart"
	// Remove indicates the intent to permanently remove a node from the Elasticsearch cluster.
	Remove ShutdownType = "remove"
)

// ShutdownStatus is the set of different status a shutdown requests can have.
type ShutdownStatus string

var (
	// ShutdownInProgress means a shutdown request has been accepted and is being processed in Elasticsearch.
	ShutdownInProgress ShutdownStatus = "IN_PROGRESS"
	// ShutdownComplete means a shutdown request has been processed and the node can be either restarted or taken out
	// of the cluster by an orchestrator.
	ShutdownComplete ShutdownStatus = "COMPLETE"
	// ShutdownStalled means a shutdown request cannot be processed further because of a limiting constraint e.g.
	// no place for shard data to migrate to.
	ShutdownStalled ShutdownStatus = "STALLED"
	// ShutdownNotStarted is an error condition that should never be returned by Elasticsearch and indicates a bug if so.
	ShutdownNotStarted ShutdownStatus = "NOT_STARTED"
)

// ShardMigration is the status of shards that are being migrated away from a node that goes through a shutdown.
type ShardMigration struct {
	Status                   ShutdownStatus `json:"status"`
	ShardMigrationsRemaining int            `json:"shard_migrations_remaining"`
	Explanation              string         `json:"explanation"`
}

// PersistentTasks expresses the status of preparing ongoing persistent tasks for a node shutdown.
type PersistentTasks struct {
	Status ShutdownStatus `json:"status"`
}

// Plugins represents the status of Elasticsearch plugins being prepared for a node shutdown.
type Plugins struct {
	Status ShutdownStatus `json:"status"`
}

// NodeShutdown is the representation of an ongoing shutdown request.
type NodeShutdown struct {
	NodeID                string          `json:"node_id"`
	Type                  string          `json:"type"`
	Reason                string          `json:"reason"`
	ShutdownStartedMillis int             `json:"shutdown_startedmillis"` // missing _ is a serialization inconsistency in Elasticsearch
	Status                ShutdownStatus  `json:"status"`
	ShardMigration        ShardMigration  `json:"shard_migration"`
	PersistentTasks       PersistentTasks `json:"persistent_tasks"`
	Plugins               Plugins         `json:"plugins"`
}

// Is tests a NodeShutdown request whether it is of type t.
func (ns NodeShutdown) Is(t ShutdownType) bool {
	// API returns type in capital letters currently
	return strings.EqualFold(ns.Type, string(t))
}

// ShutdownRequest is the body of a node shutdown request.
type ShutdownRequest struct {
	Type            ShutdownType  `json:"type"`
	Reason          string        `json:"reason"`
	AllocationDelay time.Duration `json:"allocation_delay,omitempty"`
}

// ShutdownResponse is the response wrapper for retrieving the status of ongoing node shutdowns from Elasticsearch.
type ShutdownResponse struct {
	Nodes []NodeShutdown `json:"nodes"`
}
