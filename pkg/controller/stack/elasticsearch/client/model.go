package client

import (
	"strconv"

	"github.com/elastic/stack-operators/pkg/controller/stack/common"
)

const (
	STARTED      = "STARTED"
	INITIALIZING = "INITIALIZING"
	RELOCATING   = "RELOCATING"
	UNASSIGNED   = "UNASSIGNED"
)

// Node represents an element in the `node` structure in
// Elasticsearch cluster state.
type Node struct {
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
	ClusterName  string          `json:"cluster_name"`
	ClusterUUID  string          `json:"cluster_uuid"`
	Version      int             `json:"version"`
	MasterNode   string          `json:"master_node"`
	Nodes        map[string]Node `json:"nodes"`
	RoutingTable struct {
		Indices map[string]Shards `json:"indices"`
	} `json:"routing_table"`
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

//IsRelocating is true if the shard is relocating to another node.
func (s Shard) IsRelocating() bool {
	return s.State == RELOCATING
}

//IsStarted is true if the shard is started on its current node.
func (s Shard) IsStarted() bool {
	return s.State == STARTED
}

//IsInitializing is true if the shard is currently initializing on the node.
func (s Shard) IsInitializing() bool {
	return s.State == INITIALIZING
}

//Key is a composite key of index name and shard number that identifies all
// copies of a shard across nodes.
func (s Shard) Key() string {
	return common.Concat(s.Index, "/", strconv.Itoa(s.Shard))
}

//AllocationSettings model a subset of the supported attributes for dynamic Elasticsearch cluster settings.
type AllocationSettings struct {
	ExcludeName string `json:"cluster.routing.allocation.exclude._name"`
	Enable      string `json:"cluster.routing.allocation.enable"`
} //TODO awareness settings

//ClusterRoutingAllocation models a subset of transient allocation settings for an Elasticsearch cluster.
type ClusterRoutingAllocation struct {
	Transient AllocationSettings `json:"transient"`
}
