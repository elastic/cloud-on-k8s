// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

func createTopologyElement(count int, nodeTypes v1alpha1.NodeTypesSpec) v1alpha1.TopologyElementSpec {
	return v1alpha1.TopologyElementSpec{
		NodeCount: int32(count),
		NodeTypes: nodeTypes,
	}
}

func TopologyWith(nMasters, nData, nMasterData int) []v1alpha1.TopologyElementSpec {
	var topology []v1alpha1.TopologyElementSpec
	topology = append(topology, createTopologyElement(nMasters, v1alpha1.NodeTypesSpec{
		Master: true,
	}))
	topology = append(topology, createTopologyElement(nData, v1alpha1.NodeTypesSpec{
		Data: true,
	}))
	topology = append(topology, createTopologyElement(nMasterData, v1alpha1.NodeTypesSpec{
		Master: true,
		Data:   true,
	}))
	return topology
}
