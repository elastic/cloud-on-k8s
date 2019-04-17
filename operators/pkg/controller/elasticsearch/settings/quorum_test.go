// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

func createNode(count int, nodeTypes v1alpha1.Config) v1alpha1.NodeSpec {
	return v1alpha1.NodeSpec{
		NodeCount: int32(count),
		Config:    &nodeTypes,
	}
}

func TopologyWith(nMasters, nData, nMasterData int) []v1alpha1.NodeSpec {
	var topology []v1alpha1.NodeSpec
	topology = append(topology, createNode(nMasters, v1alpha1.Config{
		Data: map[string]interface{}{
			v1alpha1.NodeMaster: "true",
		},
	}))
	topology = append(topology, createNode(nData, v1alpha1.Config{
		Data: map[string]interface{}{
			v1alpha1.NodeData:   "true",
			v1alpha1.NodeMaster: "false",
		},
	}))
	topology = append(topology, createNode(nMasterData, v1alpha1.Config{
		Data: map[string]interface{}{
			v1alpha1.NodeMaster: "true",
			v1alpha1.NodeData:   "true",
		},
	}))
	return topology
}
