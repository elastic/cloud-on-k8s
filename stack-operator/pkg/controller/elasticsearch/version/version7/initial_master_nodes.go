package version7

import (
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/label"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/settings"
	v1 "k8s.io/api/core/v1"
)

// ClusterInitialMasterNodesEnforcer enforces that cluster.initial_master_nodes is set if the cluster is bootstrapping.
func ClusterInitialMasterNodesEnforcer(
	performableChanges mutation.PerformableChanges,
	resourcesState reconcilehelper.ResourcesState,
) (*mutation.PerformableChanges, error) {
	var masterEligibleNodeNames []string
	for _, pod := range resourcesState.CurrentPods {
		if label.IsMasterNode(pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, pod.Name)
		}
	}

	// if we have masters in the cluster, we can relatively safely assume that it's already bootstrapped
	if len(masterEligibleNodeNames) > 0 {
		return &performableChanges, nil
	}

	// collect the master eligible node names from the pods we're about to create
	for _, change := range performableChanges.ToCreate {
		if label.IsMasterNode(change.Pod) {
			masterEligibleNodeNames = append(masterEligibleNodeNames, change.Pod.Name)
		}
	}

	// make every master node in the cluster aware of the others:
	for _, change := range performableChanges.ToCreate {
		if !label.IsMasterNode(change.Pod) {
			// we only need to set this on master nodes
			continue
		}

		for i, container := range change.Pod.Spec.Containers {
			container.Env = append(container.Env, v1.EnvVar{
				Name:  settings.EnvClusterInitialMasterNodes,
				Value: strings.Join(masterEligibleNodeNames, ","),
			})
			change.Pod.Spec.Containers[i] = container
		}
	}

	return &performableChanges, nil
}
