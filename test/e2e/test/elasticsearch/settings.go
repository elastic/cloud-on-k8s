// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
)

func MustNumMasterNodes(es esv1.Elasticsearch) int {
	return mustNumNodes(esv1.MasterRole, es)
}

func MustNumDataNodes(es esv1.Elasticsearch) int {
	return mustNumNodes(esv1.DataRole, es)
}

func mustNumNodes(role esv1.NodeRole, es esv1.Elasticsearch) int {
	var numNodes int
	ver := version.MustParse(es.Spec.Version)
	for _, n := range es.Spec.NodeSets {
		if hasRole(role, n, ver) {
			numNodes += int(n.Count)
		}
	}
	return numNodes
}

func hasRole(role esv1.NodeRole, node esv1.NodeSet, ver version.Version) bool {
	if node.Config == nil {
		return esv1.DefaultCfg(ver).Node.IsConfiguredWithRole(esv1.DataRole)
	}
	config, err := common.NewCanonicalConfigFrom(node.Config.Data)
	if err != nil {
		panic(err)
	}
	nodeCfg, err := settings.CanonicalConfig{
		CanonicalConfig: config,
	}.Unpack(ver)
	if err != nil {
		panic(err)
	}
	return nodeCfg.Node.HasRole(role)
}
