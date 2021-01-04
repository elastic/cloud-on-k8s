// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	"github.com/elastic/go-ucfg"
	"k8s.io/utils/pointer"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
)

const (
	NodeData                = "node.data"
	NodeIngest              = "node.ingest"
	NodeMaster              = "node.master"
	NodeML                  = "node.ml"
	NodeTransform           = "node.transform"
	NodeVotingOnly          = "node.voting_only"
	NodeRemoteClusterClient = "node.remote_cluster_client"
	NodeRoles               = "node.roles"

	MasterRole              = "master"
	DataRole                = "data"
	IngestRole              = "ingest"
	MLRole                  = "ml"
	RemoteClusterClientRole = "remote_cluster_client"
	TransformRole           = "transform"
	VotingOnlyRole          = "voting_only"
)

// ClusterSettings is the cluster node in elasticsearch.yml.
type ClusterSettings struct {
	InitialMasterNodes []string `config:"initial_master_nodes"`
}

// Node is the node section in elasticsearch.yml.
type Node struct {
	Master              *bool    `config:"master"`
	Data                *bool    `config:"data"`
	Ingest              *bool    `config:"ingest"`
	ML                  *bool    `config:"ml"`
	Transform           *bool    `config:"transform"`             // available as of 7.7.0
	RemoteClusterClient *bool    `config:"remote_cluster_client"` // available as of 7.7.0
	Roles               []string `config:"roles"`                 // available as of 7.9.0, takes priority over the other fields if non-nil
	VotingOnly          *bool    `config:"voting_only"`           // available as of 7.3.0
}

func (n *Node) HasMasterRole() bool {
	// all nodes are master-eligible by default
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.Master, true)
	}

	return stringsutil.StringInSlice(MasterRole, n.Roles)
}

func (n *Node) HasDataRole() bool {
	// all nodes are data nodes by default
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.Data, true)
	}

	return stringsutil.StringInSlice(DataRole, n.Roles)
}

func (n *Node) HasIngestRole() bool {
	// all nodes are ingest nodes by default
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.Ingest, true)
	}

	return stringsutil.StringInSlice(IngestRole, n.Roles)
}

func (n *Node) HasMLRole() bool {
	// all nodes are ML nodes by default
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.ML, true)
	}

	return stringsutil.StringInSlice(MLRole, n.Roles)
}

func (n *Node) HasRemoteClusterClientRole() bool {
	// all nodes are remote_cluster_client nodes by default
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.RemoteClusterClient, true)
	}

	return stringsutil.StringInSlice(RemoteClusterClientRole, n.Roles)
}

func (n *Node) HasTransformRole() bool {
	// all nodes are data nodes by default and data nodes are transform nodes by default as well.
	if n == nil {
		return true
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.Transform, n.HasDataRole())
	}

	return stringsutil.StringInSlice(TransformRole, n.Roles)
}

func (n *Node) HasVotingOnlyRole() bool {
	// voting only is not enabled by default
	if n == nil {
		return false
	}

	if n.Roles == nil {
		return pointer.BoolPtrDerefOr(n.VotingOnly, false)
	}

	return stringsutil.StringInSlice(VotingOnlyRole, n.Roles)
}

// ElasticsearchSettings is a typed subset of elasticsearch.yml for purposes of the operator.
type ElasticsearchSettings struct {
	Node    *Node           `config:"node"`
	Cluster ClusterSettings `config:"cluster"`
}

// DefaultCfg is an instance of ElasticsearchSettings with defaults set as they are in Elasticsearch.
// cfg is the user provided config we want defaults for, ver is the version of Elasticsearch.
func DefaultCfg(ver version.Version) ElasticsearchSettings {
	settings := ElasticsearchSettings{
		// Values below only make sense if there is no "node.roles" in the configuration provided by the user
		Node: &Node{
			Master: pointer.BoolPtr(true),
			Data:   pointer.BoolPtr(true),
			Ingest: pointer.BoolPtr(true),
			ML:     pointer.BoolPtr(true),
		},
	}

	configureTransformRole(&settings, ver)

	return settings
}

// UnpackConfig unpacks Config into a typed subset.
func UnpackConfig(c *commonv1.Config, ver version.Version, out *ElasticsearchSettings) error {
	if c == nil {
		return nil
	}

	config, err := ucfg.NewFrom(c.Data, commonv1.CfgOptions...)
	if err != nil {
		return err
	}

	if err := config.Unpack(out, commonv1.CfgOptions...); err != nil {
		return err
	}

	configureTransformRole(out, ver)

	return nil
}

// configureTransformRole explicitly sets the transform role to false if the version is below 7.7.0
func configureTransformRole(cfg *ElasticsearchSettings, ver version.Version) {
	// nothing to do if the version is above 7.7.0 as the transform role is automatically applied to data nodes by the HasTransformRole method.
	if ver.IsSameOrAfter(version.From(7, 7, 0)) {
		return
	}

	if cfg.Node == nil {
		cfg.Node = &Node{}
	}

	cfg.Node.Transform = pointer.BoolPtr(false)
}
