package client

import "github.com/elastic/stack-operators/pkg/controller/stack/common"

const (
	STARTED = "STARTED"
	INITIALIZING = "INITIALIZING"
	RELOCATING = "RELOCATING"

)

type Shard struct {
	Index  string `json:"index"`
	Shard  string `json:"shard"`
	Prirep string `json:"prirep"`
	State  string `json:"state"`
	Store  string `json:"string"`
	IP     string `json:"ip"`
	Node   string `json:"node"`
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
	return common.Concat(s.Index, "/", s.Shard)
}

type TransientSettings struct {
	ExcludeName string `json:"cluster.routing.allocation.exclude._name"`
	Enable      string `json:"cluster.routing.allocation.enable"`
} //TODO awareness settings

type ClusterRoutingAllocation struct {
	Transient TransientSettings `json:"transient"`
}