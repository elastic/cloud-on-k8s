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

// Shard models a hybrid of _cat/shards shard and routing table shard
// Node is the node name from cluster state as in _cat/shards
// but it will never contain any relocation information.
// Primary is a boolean as in cluster state.
type Shard struct {
	Index   string `json:"index"`
	Shard   int    `json:"shard"`
	Primary bool   `json:"primary"`
	State   string `json:"state"`
	Node    string `json:"node"`
}

func (s Shard) IsRelocating() bool {
	return s.State == RELOCATING
}
func (s Shard) IsStarted() bool {
	return s.State == STARTED
}

func (s Shard) IsInitializing() bool {
	return s.State == INITIALIZING
}

func (s Shard) Key() string {
	return common.Concat(s.Index, "/", strconv.Itoa(s.Shard))
}

type TransientSettings struct {
	ExcludeName string `json:"cluster.routing.allocation.exclude._name"`
	Enable      string `json:"cluster.routing.allocation.enable"`
} //TODO awareness settings

type ClusterRoutingAllocation struct {
	Transient TransientSettings `json:"transient"`
}
