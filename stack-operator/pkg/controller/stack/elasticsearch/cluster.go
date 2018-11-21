package elasticsearch

import deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given topology
func ComputeMinimumMasterNodes(topologies []deploymentsv1alpha1.ElasticsearchTopologySpec) int {
	nMasters := 0
	for _, t := range topologies {
		if t.NodeTypes.Master {
			nMasters += int(t.NodeCount)
		}
	}
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}
