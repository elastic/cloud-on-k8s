// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import (
	"encoding/json"

	"github.com/elastic/go-ucfg"
)

const (
	NodeData   = "node.data"
	NodeIngest = "node.ingest"
	NodeMaster = "node.master"
	NodeML     = "node.ml"
)

// ClusterSettings is the cluster node in elasticsearch.yml.
type ClusterSettings struct {
	InitialMasterNodes []string `config:"initial_master_nodes"`
}

// Node is the node node in elasticsearch.yml.
type Node struct {
	Master bool `config:"master"`
	Data   bool `config:"data"`
	Ingest bool `config:"ingest"`
	ML     bool `config:"ml"`
}

// ElasticsearchSettings is a typed subset of elasticsearch.yml for purposes of the operator.
type ElasticsearchSettings struct {
	Node    Node            `config:"node"`
	Cluster ClusterSettings `config:"cluster"`
}

// DefaultCfg is an instance of ElasticsearchSettings with defaults set as they are in elasticsearch.yml.
var DefaultCfg = ElasticsearchSettings{
	Node: Node{
		Master: true,
		Data:   true,
		Ingest: true,
		ML:     true,
	},
}

// CfgOptions are config options for elasticsearch.yml. Currently contains only support for dotted keys.
var CfgOptions = []ucfg.Option{ucfg.PathSep(".")}

// Config represents untyped elasticsearch.yml configuration inside the Elasticsearch spec.
type Config struct {
	// This field exists to work around https://github.com/kubernetes-sigs/kubebuilder/issues/528
	Data map[string]interface{}
}

// NewConfig constructs a Config with the given unstructured configuration data.
func NewConfig(cfg map[string]interface{}) Config {
	return Config{Data: cfg}
}

// MarshalJSON implements the Marshaler interface.
func (c *Config) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Data)
}

// UnmarshalJSON implements the Unmarshaler interface.
func (c *Config) UnmarshalJSON(data []byte) error {
	var out map[string]interface{}
	err := json.Unmarshal(data, &out)
	if err != nil {
		return err
	}
	c.Data = out
	return nil
}

// DeepCopyInto is an ~autogenerated~ deepcopy function, copying the receiver, writing into out. in must be non-nil.
// This exists here to work around https://github.com/kubernetes/code-generator/issues/50
func (c *Config) DeepCopyInto(out *Config) {
	bytes, err := json.Marshal(c.Data)
	if err != nil {
		// we assume that it marshals cleanly because otherwise the resource would not have been
		// created in the API server
		panic(err)
	}
	var copy map[string]interface{}
	err = json.Unmarshal(bytes, &copy)
	if err != nil {
		// we assume again optimistically that because we just marshalled that the round trip works as well
		panic(err)
	}
	out.Data = copy
	return
}

// MustUnpack returns a typed subset of the Config.
// Panics on errors.
func (c Config) MustUnpack() ElasticsearchSettings {
	cfg, err := c.Unpack()
	if err != nil {
		panic(err)
	}
	return cfg
}

// Unpack unpacks Config into a typed subset.
func (c Config) Unpack() (ElasticsearchSettings, error) {
	esSettings := DefaultCfg // defensive copy
	config, err := ucfg.NewFrom(c.Data, CfgOptions...)
	if err != nil {
		return esSettings, err
	}
	err = config.Unpack(&esSettings, CfgOptions...)
	return esSettings, err
}
