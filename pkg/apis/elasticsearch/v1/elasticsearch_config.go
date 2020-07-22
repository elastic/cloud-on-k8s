// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/go-ucfg"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	NodeData      = "node.data"
	NodeIngest    = "node.ingest"
	NodeMaster    = "node.master"
	NodeML        = "node.ml"
	NodeTransform = "node.transform"

	MasterRole    = "master"
	DataRole      = "data"
	IngestRole    = "ingest"
	MLRole        = "ml"
	TransformRole = "transform"
)

// ClusterSettings is the cluster node in elasticsearch.yml.
type ClusterSettings struct {
	InitialMasterNodes []string `config:"initial_master_nodes"`
}

// Node is the node section in elasticsearch.yml.
type Node struct {
	Master    bool     `config:"master"`
	Data      bool     `config:"data"`
	Ingest    bool     `config:"ingest"`
	ML        bool     `config:"ml"`
	Transform bool     `config:"transform"` // available as of 7.7.0
	Roles     []string `config:"roles"`     // available as of 7.9.0, takes priority over the other fields if non-nil
}

func (n *Node) HasMasterRole() bool {
	if n.Roles == nil {
		return n.Master
	}
	return stringsutil.StringInSlice(MasterRole, n.Roles)
}

func (n *Node) HasDataRole() bool {
	if n.Roles == nil {
		return n.Data
	}
	return stringsutil.StringInSlice(DataRole, n.Roles)
}

func (n *Node) HasIngestRole() bool {
	if n.Roles == nil {
		return n.Ingest
	}
	return stringsutil.StringInSlice(IngestRole, n.Roles)
}

func (n *Node) HasMLRole() bool {
	if n.Roles == nil {
		return n.ML
	}
	return stringsutil.StringInSlice(MLRole, n.Roles)
}

func (n *Node) HasTransformRole() bool {
	if n.Roles == nil {
		return n.Transform
	}
	return stringsutil.StringInSlice(TransformRole, n.Roles)
}

// ElasticsearchSettings is a typed subset of elasticsearch.yml for purposes of the operator.
type ElasticsearchSettings struct {
	Node    Node            `config:"node"`
	Cluster ClusterSettings `config:"cluster"`
}

// DefaultCfg is an instance of ElasticsearchSettings with defaults set as they are in Elasticsearch.
// cfg is the user provided config we want defaults for, ver is the version of Elasticsearch.
func DefaultCfg(cfg *ucfg.Config, ver version.Version) ElasticsearchSettings {
	settings := ElasticsearchSettings{
		// Values below only make sense if there is no "node.roles" in the configuration provided by the user
		Node: Node{
			Master:    true,
			Data:      true,
			Ingest:    true,
			ML:        true,
			Transform: false,
		},
	}
	if ver.IsSameOrAfter(version.From(7, 7, 0)) {
		// this setting did not exist before 7.7.0 its default depends on the node.data value
		settings.Node.Transform = true
		if cfg == nil {
			return settings
		}
		dataNode, err := cfg.Bool(NodeData, -1, commonv1.CfgOptions...)
		if err == nil && !dataNode {
			settings.Node.Transform = false
		}
	}
	return settings
}

// UnpackConfig unpacks Config into a typed subset.
func UnpackConfig(c *commonv1.Config, ver version.Version) (ElasticsearchSettings, error) {
	if c == nil {
		// make this nil safe to allow a ptr value to work around Json serialization issues
		return DefaultCfg(ucfg.New(), ver), nil
	}
	config, err := ucfg.NewFrom(c.Data, commonv1.CfgOptions...)
	if err != nil {
		return ElasticsearchSettings{}, err
	}
	esSettings := DefaultCfg(config, ver)
	err = config.Unpack(&esSettings, commonv1.CfgOptions...)
	return esSettings, err
}
