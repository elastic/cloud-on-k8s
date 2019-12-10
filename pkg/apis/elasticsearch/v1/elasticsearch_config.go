// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"github.com/elastic/go-ucfg"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
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

// Node is the node section in elasticsearch.yml.
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

// DefaultCfg is an instance of ElasticsearchSettings with defaults set as they are in Elasticsearch.
var DefaultCfg = ElasticsearchSettings{
	Node: Node{
		Master: true,
		Data:   true,
		Ingest: true,
		ML:     true,
	},
}

// Unpack unpacks Config into a typed subset.
func UnpackConfig(c *commonv1.Config) (ElasticsearchSettings, error) {
	esSettings := DefaultCfg // defensive copy
	if c == nil {
		// make this nil safe to allow a ptr value to work around Json serialization issues
		return esSettings, nil
	}
	config, err := ucfg.NewFrom(c.Data, commonv1.CfgOptions...)
	if err != nil {
		return esSettings, err
	}
	err = config.Unpack(&esSettings, commonv1.CfgOptions...)
	return esSettings, err
}
