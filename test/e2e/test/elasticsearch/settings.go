// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
)

func MustNumDataNodes(es esv1.Elasticsearch) int {
	var numNodes int
	ver := version.MustParse(es.Spec.Version)
	for _, n := range es.Spec.NodeSets {
		if isDataNode(n, ver) {
			numNodes += int(n.Count)
		}
	}
	return numNodes
}

func isDataNode(node esv1.NodeSet, ver version.Version) bool {
	if node.Config == nil {
		return esv1.DefaultCfg(ver).Node.HasDataRole()
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
	return nodeCfg.Node.HasDataRole()
}
