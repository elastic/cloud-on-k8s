// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/go-ucfg"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	NodeData      = "node.data"
	NodeIngest    = "node.ingest"
	NodeMaster    = "node.master"
	NodeML        = "node.ml"
	NodeTransform = "node.transform"
)

// ClusterSettings is the cluster node in elasticsearch.yml.
type ClusterSettings struct {
	InitialMasterNodes []string `config:"initial_master_nodes"`
}

// Node is the node section in elasticsearch.yml.
type Node struct {
	Master    bool `config:"master"`
	Data      bool `config:"data"`
	Ingest    bool `config:"ingest"`
	ML        bool `config:"ml"`
	Transform bool `config:transform` // available as of 7.7.0
}

// ElasticsearchSettings is a typed subset of elasticsearch.yml for purposes of the operator.
type ElasticsearchSettings struct {
	Node    Node            `config:"node"`
	Cluster ClusterSettings `config:"cluster"`
}

// DefaultCfg is an instance of ElasticsearchSettings with defaults set as they are in Elasticsearch.
func DefaultCfg(ver version.Version) ElasticsearchSettings {
	settings := ElasticsearchSettings{
		Node: Node{
			Master:    true,
			Data:      true,
			Ingest:    true,
			ML:        true,
			Transform: true,
		},
	}
	if !ver.IsSameOrAfter(version.From(7, 7, 0)) {
		// this setting did not exist before 7.7.0 expressed here by setting it to false this allows us to keep working with just one model
		settings.Node.Transform = false
	}
	return settings
}

// Unpack unpacks Config into a typed subset.
func UnpackConfig(c *commonv1.Config, ver version.Version) (ElasticsearchSettings, error) {
	esSettings := DefaultCfg(ver)
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
