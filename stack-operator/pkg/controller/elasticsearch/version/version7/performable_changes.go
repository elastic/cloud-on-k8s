package version7

import (
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"k8s.io/api/core/v1"
)

// ClusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set if the cluster is bootstrapping.
func ClusterInitialMasterNodesEnforcer(
	performableChanges mutation.PerformableChanges,
	resourcesState support.ResourcesState,
) (*mutation.PerformableChanges, error) {
	// if no masters in the cluster, it's bootstrapping
	var masterEligibleNodeNames []string
	for _, pod := range resourcesState.CurrentPods {
		if support.IsMasterNode(pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, pod.Name)
		}
	}
	shouldSetInitialMasters := len(masterEligibleNodeNames) == 0
	if shouldSetInitialMasters {
		for _, change := range performableChanges.ToCreate {
			if support.IsMasterNode(change.Pod) {
				masterEligibleNodeNames = append(masterEligibleNodeNames, change.Pod.Name)
			}
		}
	}

	for j, change := range performableChanges.ToCreate {
		for i, container := range change.Pod.Spec.Containers {
			container.Env = append(container.Env, v1.EnvVar{
				Name:  support.EnvClusterInitialMasterNodes,
				Value: strings.Join(masterEligibleNodeNames, ","),
			})
			change.Pod.Spec.Containers[i] = container
		}
		// TODO: is this required?
		performableChanges.ToCreate[j] = change
	}

	return &performableChanges, nil
}
