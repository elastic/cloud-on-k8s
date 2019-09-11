// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"encoding/json"
	"strconv"
	"time"

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
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	ActiveShards                int     `json:"active_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int     `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int     `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float32 `json:"active_shards_percent_as_number"`
}

// These are possible shard states
const (
	STARTED      = "STARTED"
	INITIALIZING = "INITIALIZING"
	RELOCATING   = "RELOCATING"
	UNASSIGNED   = "UNASSIGNED"
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
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
	JVM   struct {
		StartTimeInMillis int64 `json:"start_time_in_millis"`
		Mem               struct {
			HeapMaxInBytes int `json:"heap_max_in_bytes"`
		} `json:"mem"`
	} `json:"jvm"`
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

// Shards contains the shards in the Elasticsearch routing table
// mapped to their shard number.
type Shards struct {
	Shards map[string][]Shard `json:"shards"`
}

// ClusterState partially models Elasticsearch cluster state.
type ClusterState struct {
	ClusterName  string                      `json:"cluster_name"`
	ClusterUUID  string                      `json:"cluster_uuid"`
	Version      int                         `json:"version"`
	MasterNode   string                      `json:"master_node"`
	Nodes        map[string]ClusterStateNode `json:"nodes"`
	RoutingTable RoutingTable                `json:"routing_table"`
}

type RoutingTable struct {
	Indices map[string]Shards `json:"indices"`
}

// IsEmpty returns true if this is an empty struct without data.
func (cs ClusterState) IsEmpty() bool {
	return cs.ClusterName == "" &&
		cs.ClusterUUID == "" &&
		cs.Version == 0 &&
		cs.MasterNode == "" &&
		len(cs.Nodes) == 0 &&
		len(cs.RoutingTable.Indices) == 0
}

// GetShards reads all shards from cluster state,
// similar to what _cat/shards does but it is consistent in
// its output.
func (cs ClusterState) GetShards() []Shard {
	var result []Shard
	for _, index := range cs.RoutingTable.Indices {
		for _, shards := range index.Shards {
			for _, shard := range shards {
				shard.Node = cs.Nodes[shard.Node].Name
				result = append(result, shard)
			}
		}
	}
	return result
}

// GetShardsByNode returns shards by node.
// The result is a map with the name of the nodes as keys and the list of shards on the nodes as values.
func (cs ClusterState) GetShardsByNode() map[string][]Shard {
	result := make(map[string][]Shard)
	for _, index := range cs.RoutingTable.Indices {
		for _, shards := range index.Shards {
			// for each shard, check if it assigned to a node
			for _, shard := range shards {
				if len(shard.Node) == 0 {
					continue
				}
				// shard.Node is the id of the node, get the corresponding node name
				nodeName := cs.Nodes[shard.Node].Name
				result[nodeName] = append(result[nodeName], shard)
			}
		}
	}
	return result
}

// MasterNodeName is the name of the current master node in the Elasticsearch cluster.
func (cs ClusterState) MasterNodeName() string {
	return cs.Nodes[cs.MasterNode].Name
}

// NodesByNodeName returns the Nodes indexed by their Node.Name instead of their Node ID.
func (cs ClusterState) NodesByNodeName() map[string]ClusterStateNode {
	nodesByName := make(map[string]ClusterStateNode, len(cs.Nodes))
	for _, node := range cs.Nodes {
		nodesByName[node.Name] = node
	}
	return nodesByName
}

// Shard models a hybrid of _cat/shards shard and routing table shard.
type Shard struct {
	Index string `json:"index"`
	Shard int    `json:"shard"`
	// Primary is a boolean as in cluster state.
	Primary bool   `json:"primary"`
	State   string `json:"state"`
	// Node is the node name not the Node id
	Node string `json:"node"`
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

// Key is a composite key of index name and shard number that identifies all
// copies of a shard across nodes.
func (s Shard) Key() string {
	return stringsutil.Concat(s.Index, "/", strconv.Itoa(s.Shard))
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

// ErrorResponse is a Elasticsearch error response.
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

// License models the Elasticsearch license applied to a cluster. Signature will be empty on reads. IssueDate,  ExpiryTime and Status can be empty on writes.
type License struct {
	Status             string     `json:"status,omitempty"`
	UID                string     `json:"uid"`
	Type               string     `json:"type"`
	IssueDate          *time.Time `json:"issue_date,omitempty"`
	IssueDateInMillis  int64      `json:"issue_date_in_millis"`
	ExpiryDate         *time.Time `json:"expiry_date,omitempty"`
	ExpiryDateInMillis int64      `json:"expiry_date_in_millis"`
	MaxNodes           int        `json:"max_nodes"`
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
	return lr.LicenseStatus == "valid"
}

// LicenseResponse is the response to GET _xpack/license. Licenses won't contain signature.
type LicenseResponse struct {
	License License `json:"license"`
}

// Settings is the root element of settings.
type Settings struct {
	PersistentSettings *SettingsGroup `json:"persistent,omitempty"`
	TransientSettings  *SettingsGroup `json:"transient,omitempty"`
}

// SettingsGroup is a group of settings, either to transient or persistent.
type SettingsGroup struct {
	Cluster Cluster `json:"cluster,omitempty"`
}

// Cluster models the configuration of the cluster.
type Cluster struct {
	RemoteClusters map[string]RemoteCluster `json:"remote,omitempty"`
}

// RemoteClusterSeeds is the set of seeds to use in a remote cluster setting.
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
