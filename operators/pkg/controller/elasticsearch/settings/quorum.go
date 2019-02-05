package settings

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func quorum(nMasters int) int {
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}

// ComputeMinimumMasterNodes returns the minimum number of master nodes
// that should be set in a cluster with the given topology
func ComputeMinimumMasterNodes(topologies []v1alpha1.ElasticsearchTopologySpec) int {
	nMasters := 0
	for _, t := range topologies {
		if t.NodeTypes.Master {
			nMasters += int(t.NodeCount)
		}
	}
	return quorum(nMasters)
}

// ComputeMinimumMasterNodesFromPods returns the minimum number of master nodes based on the
// current topology of the cluster.
func ComputeMinimumMasterNodesFromPods(cluster []corev1.Pod) int {
	nMasters := 0
	for _, p := range cluster {
		for _, c := range p.Spec.Containers {
			for _, e := range c.Env {
				if e.Name == EnvNodeMaster && e.Value == "true" {
					nMasters++
				}
			}
		}
	}
	return quorum(nMasters)
}
