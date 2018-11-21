package client

import (
	"strconv"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
)

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

// EndedBefore returns true if the snapshot has and end time and that end time is further in the
// past relativ to the given now than duration d.
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
