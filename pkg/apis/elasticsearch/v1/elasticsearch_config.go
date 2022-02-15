// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"github.com/elastic/go-ucfg"
	"k8s.io/utils/pointer"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

type NodeRole string

const (
	DataColdRole            NodeRole = "data_cold"
	DataContentRole         NodeRole = "data_content"
	DataFrozenRole          NodeRole = "data_frozen"
	DataHotRole             NodeRole = "data_hot"
	DataRole                NodeRole = "data"
	DataWarmRole            NodeRole = "data_warm"
	IngestRole              NodeRole = "ingest"
	MLRole                  NodeRole = "ml"
	MasterRole              NodeRole = "master"
	RemoteClusterClientRole NodeRole = "remote_cluster_client"
	TransformRole           NodeRole = "transform"
	VotingOnlyRole          NodeRole = "voting_only"
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

// CanContainData returns true if a node can contain data, it returns false otherwise.
func (n *Node) CanContainData() bool {
	return n.HasRole(DataRole) ||
		n.HasRole(DataHotRole) ||
		n.HasRole(DataWarmRole) ||
		n.HasRole(DataColdRole) ||
		n.HasRole(DataFrozenRole) ||
		n.HasRole(DataContentRole)
}

// HasRole returns true if the node runs with the given role.
func (n *Node) HasRole(role NodeRole) bool {
	switch role {
	case DataContentRole:
		return n.IsConfiguredWithRole(DataRole) || n.IsConfiguredWithRole(DataContentRole)
	case DataHotRole:
		return n.IsConfiguredWithRole(DataRole) || n.IsConfiguredWithRole(DataHotRole)
	case DataWarmRole:
		return n.IsConfiguredWithRole(DataRole) || n.IsConfiguredWithRole(DataWarmRole)
	case DataColdRole:
		return n.IsConfiguredWithRole(DataRole) || n.IsConfiguredWithRole(DataColdRole)
	case DataFrozenRole:
		return n.IsConfiguredWithRole(DataRole) || n.IsConfiguredWithRole(DataFrozenRole)
	default:
		return n.IsConfiguredWithRole(role)
	}
}

// DependsOn returns true if a tier should be upgraded before another one.
func (n *Node) DependsOn(other *Node) bool {
	switch {
	case !n.HasRole(MasterRole) && other.HasRole(MasterRole):
		// other might be a dependency, but it is also a master node. We don't want to enter a deadlock where other is
		// the last master node, while the candidate is not and must be upgraded first.
		return false
	case n.HasRole(DataHotRole):
		// hot tier must be upgraded after warm, cold and frozen
		return other.HasRole(DataWarmRole) || other.HasRole(DataColdRole) || other.HasRole(DataFrozenRole)
	case n.HasRole(DataWarmRole):
		// warm tier must be upgraded after cold and frozen
		return other.HasRole(DataColdRole) || other.HasRole(DataFrozenRole)
	case n.HasRole(DataColdRole):
		// cold tier must be upgraded after frozen
		return other.HasRole(DataFrozenRole)
	}
	// frozen and content have no dependency
	return false
}

// IsConfiguredWithRole returns true if the node has the given role in its configuration.
func (n *Node) IsConfiguredWithRole(role NodeRole) bool {
	if n == nil {
		// Nodes have all the roles by default except for the voting_only role.
		return role != VotingOnlyRole
	}

	if n.Roles != nil {
		return stringsutil.StringInSlice(string(role), n.Roles)
	}

	switch role {
	case DataRole:
		return pointer.BoolPtrDerefOr(n.Data, true)
	case DataFrozenRole, DataColdRole, DataContentRole, DataHotRole, DataWarmRole:
		// These roles should really be defined in node.roles. Since they were not, assume they are enabled unless node.data is set to false.
		return pointer.BoolPtrDerefOr(n.Data, true)
	case IngestRole:
		return pointer.BoolPtrDerefOr(n.Ingest, true)
	case MLRole:
		return pointer.BoolPtrDerefOr(n.ML, true)
	case MasterRole:
		return pointer.BoolPtrDerefOr(n.Master, true)
	case RemoteClusterClientRole:
		return pointer.BoolPtrDerefOr(n.RemoteClusterClient, true)
	case TransformRole:
		// all data nodes are transform nodes by default as well.
		return pointer.BoolPtrDerefOr(n.Transform, n.IsConfiguredWithRole(DataRole))
	case VotingOnlyRole:
		return pointer.BoolPtrDerefOr(n.VotingOnly, false)
	}

	// This point should never be reached. The default is to assume that a node has all roles except voting_only.
	return role != VotingOnlyRole
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
	if ver.GTE(version.From(7, 7, 0)) {
		return
	}

	if cfg.Node == nil {
		cfg.Node = &Node{}
	}

	cfg.Node.Transform = pointer.BoolPtr(false)
}
