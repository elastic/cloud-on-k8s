package client

import (
	"strconv"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
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

// Node partially models an Elasticsearch node retrieved from /_nodes
type Node struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
	JVM   struct {
		Mem struct {
			HeapMaxInBytes int `json:"heap_max_in_bytes"`
		} `json:"mem"`
	} `json:"jvm"`
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
	RoutingTable struct {
		Indices map[string]Shards `json:"indices"`
	} `json:"routing_table"`
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
	return common.Concat(s.Index, "/", strconv.Itoa(s.Shard))
}

// AllocationSettings model a subset of the supported attributes for dynamic Elasticsearch cluster settings.
type AllocationSettings struct {
	ExcludeName string `json:"cluster.routing.allocation.exclude._name"`
	Enable      string `json:"cluster.routing.allocation.enable"`
} // TODO awareness settings

// ClusterRoutingAllocation models a subset of transient allocation settings for an Elasticsearch cluster.
type ClusterRoutingAllocation struct {
	Transient AllocationSettings `json:"transient"`
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

// SnapshotRepositorySetttings is the settings section of the repository definition. Provider specific.
type SnapshotRepositorySetttings struct {
	Bucket string `json:"bucket"`
	Client string `json:"client"`
}

// SnapshotRepository partially models Elasticsearch repository settings.
type SnapshotRepository struct {
	Type     string                      `json:"type"`
	Settings SnapshotRepositorySetttings `json:"settings"`
}

// SnapshotStates as in Elasticsearch.
const (
	SnapshotStateSuccess      = "SUCCESS"
	SnapshotStateFailed       = "FAILED"
	SnapshotStateInProgress   = "IN_PROGRESS"
	SnapshotStatePartial      = "PARTIAL"
	SnapshotStateIncompatible = "INCOMPATIBLE"
)

// Snapshot represents a single snapshot.
type Snapshot struct {
	Snapshot          string        `json:"snapshot"`
	UUID              string        `json:"uuid"`
	VersionID         int           `json:"version_id"`
	Version           string        `json:"version"`
	Indices           []string      `json:"indices"`
	State             string        `json:"state"`
	StartTime         time.Time     `json:"start_time"`
	StartTimeInMillis int64         `json:"start_time_in_millis"`
	EndTime           time.Time     `json:"end_time"`
	EndTimeInMillis   int64         `json:"end_time_in_millis"`
	DurationInMillis  int           `json:"duration_in_millis"`
	Failures          []interface{} `json:"failures"`
	Shards            struct {
		Total      int `json:"total"`
		Failed     int `json:"failed"`
		Successful int `json:"successful"`
	} `json:"shards"`
}

// IsSuccess is true when the snapshot succeeded.
func (s Snapshot) IsSuccess() bool {
	return s.State == SnapshotStateSuccess
}

// IsFailed is true if the snapshot failed.
func (s Snapshot) IsFailed() bool {
	return s.State == SnapshotStateFailed
}

// IsInProgress is true if the snapshot is still running.
func (s Snapshot) IsInProgress() bool {
	return s.State == SnapshotStateInProgress
}

// IsPartial is true if only a subset of indices could be snapshotted.
func (s Snapshot) IsPartial() bool {
	return s.State == SnapshotStatePartial
}

// IsIncompatible is true if the snapshot was taken with an incompatible
// version of Elasticsearch compared to the currently running version.
func (s Snapshot) IsIncompatible() bool {
	return s.State == SnapshotStateIncompatible
}

// IsComplete is true when a snapshot has reached one of its end states.
func (s Snapshot) IsComplete() bool {
	return s.IsSuccess() || s.IsFailed() || s.IsPartial()
}

// EndedBefore returns true if the snapshot has an end time and that end time is further in the
// past relative to the given now than duration d.
func (s Snapshot) EndedBefore(d time.Duration, now time.Time) bool {
	if s.EndTime.IsZero() {
		return false
	}
	return now.Sub(s.EndTime) > d
}

// SnapshotsList models the list response from the _snaphshot API.
type SnapshotsList struct {
	Snapshots []Snapshot `json:"snapshots"`
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

// License models the Elasticsearch license applied to a cluster (signature can be empty dependin)
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
