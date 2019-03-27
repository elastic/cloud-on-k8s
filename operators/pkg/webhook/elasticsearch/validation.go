// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"

// HasMaster checks if the given Elasticsearch cluster has at least one master node.
func HasMaster(esCluster v1alpha1.Elasticsearch) bool {
	var hasMaster bool
	for _, t := range esCluster.Spec.Topology {
		hasMaster = hasMaster || (t.NodeTypes.Master && t.NodeCount > 0)
	}
	return hasMaster
}
