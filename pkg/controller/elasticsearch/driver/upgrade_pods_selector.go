// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"sort"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	corev1 "k8s.io/api/core/v1"
)

type PredicateContext struct {
	masterNodesNames []string
	healthyPods      map[string]corev1.Pod
	toUpdate         []corev1.Pod
	esState          ESState
}

// Predicate is a function that indicates if a Pod can be deleted (or not).
type Predicate struct {
	name string
	fn   func(context PredicateContext, candidate corev1.Pod, deletedPods []corev1.Pod, maxUnavailableReached bool) (bool, error)
}

func NewPredicateContext(
	state ESState,
	healthyPods map[string]corev1.Pod,
	podsToUpgrade []corev1.Pod,
	masterNodesNames []string,
) PredicateContext {
	return PredicateContext{
		masterNodesNames: masterNodesNames,
		healthyPods:      healthyPods,
		toUpdate:         podsToUpgrade,
		esState:          state,
	}
}

// sortCandidates is the default sort function, masters have lower priority as
// we want to update the data nodes first.
// If 2 Pods are of the same type then use the reverse ordinal order.
// TODO: Add some priority to unhealthy (bootlooping) Pods
func sortCandidates(allPods []corev1.Pod) {
	sort.Slice(allPods, func(i, j int) bool {
		pod1 := allPods[i]
		pod2 := allPods[j]
		if (label.IsMasterNode(pod1) && label.IsMasterNode(pod2)) ||
			(!label.IsMasterNode(pod1) && !label.IsMasterNode(pod2)) { // same type, use the reverse name function
			ssetName1, ord1, err := sset.StatefulSetName(pod1.Name)
			if err != nil {
				return false
			}
			ssetName2, ord2, err := sset.StatefulSetName(pod2.Name)
			if err != nil {
				return false
			}
			if ssetName1 == ssetName2 {
				// same name, compare ordinal, higher first
				return ord1 > ord2
			}
			return ssetName1 < ssetName2
		}
		if label.IsMasterNode(pod1) && !label.IsMasterNode(pod2) {
			// pod2 has higher priority since it is a data node
			return false
		}
		return true
	})
}

var predicates = [...]Predicate{
	{
		// If MaxUnavailable is reached, allow for an unhealthy Pod to be deleted.
		// This is to prevent a situation where MaxUnavailable is reached and we
		// can't make progress even if the user has updated the spec.
		name: "do_not_restart_healthy_node_if_MaxUnavailable_reached",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			_, healthy := context.healthyPods[candidate.Name]
			if maxUnavailableReached && healthy {
				return false, nil
			}
			return true, nil
		},
	},
	{
		name: "skip_already_terminating_pods",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			if candidate.DeletionTimestamp != nil {
				// Pod is already terminating, skip it
				return false, nil
			}
			return true, nil
		},
	},
	{
		// In Yellow or Red status only allow unhealthy Pods to be restarted.
		// This is intended to unlock some situations where the cluster is not green and
		// a Pod has to be restarted a second time.
		name: "do_not_restart_healthy_node_if_not_green",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			// Green health is retrieved only once from the cluster.
			// We rely on "shard conflict" predicate to avoid to delete to ES nodes that share some shards.
			green, err := context.esState.GreenHealth()
			if err != nil {
				return false, err
			}
			if green {
				return true, nil
			}
			_, healthy := context.healthyPods[candidate.Name]
			if !healthy {
				return true, nil
			}
			return false, nil
		},
	},
	{
		// One master at a time
		name: "one_master_at_a_time",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			// If candidate is not a master then we don't care
			if !label.IsMasterNode(candidate) {
				return true, nil
			}
			// If candidate is not healthy we want to give it a chance to restart
			_, healthy := context.healthyPods[candidate.Name]
			if !healthy {
				return true, nil
			}

			for _, pod := range deletedPods {
				if label.IsMasterNode(pod) {
					return false, nil
				}
			}
			// Get the expected masters
			expectedMasters := len(context.masterNodesNames)
			// Get the healthy masters
			healthyMasters := 0
			for _, pod := range context.healthyPods {
				if label.IsMasterNode(pod) {
					healthyMasters++
				}
			}
			// We are relying here on the expectations that give us the guarantee
			// that there is no upscale or downscale in progress.
			return healthyMasters == expectedMasters, nil
		},
	},
	{
		// Force an upgrade of all the data nodes before upgrading the last master
		name: "do_not_delete_last_master_if_data_nodes_are_not_upgraded",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			// If candidate is not a master then we don't care
			if !label.IsMasterNode(candidate) {
				return true, nil
			}
			for _, pod := range context.toUpdate {
				if candidate.Name == pod.Name {
					continue
				}
				if label.IsMasterNode(pod) {
					// There are some other masters alive, allow this one to be deleted
					return true, nil
				}
			}
			// This is the last master, check if all data nodes are up to date
			for _, pod := range context.toUpdate {
				if candidate.Name == pod.Name {
					continue
				}
				if label.IsDataNode(pod) {
					// There's still a data node to update
					return false, nil
				}
			}
			return true, nil
		},
	},
	{
		// We should not delete 2 Pods with the same shards
		name: "do_not_delete_pods_with_same_shards",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			if len(deletedPods) == 0 {
				// Do not do unnecessary request
				return true, nil
			}
			clusterState, err := context.esState.GetClusterState()
			if err != nil {
				return false, err
			}
			shards := clusterState.GetShardsByNode()
			shardsOnCandidate, ok := shards[candidate.Name]
			if !ok {
				// No shards on this node
				return true, nil
			}

			for _, deletedPod := range deletedPods {
				shardsOnDeletedPod, ok := shards[deletedPod.Name]
				if !ok {
					// No shards on the deleted pod
					continue
				}
				if conflictingShards(shardsOnCandidate, shardsOnDeletedPod) {
					return false, nil
				}
			}
			return true, nil
		},
	},
}

func conflictingShards(shards1, shards2 []client.Shard) bool {
	for _, shards1 := range shards1 {
		for _, shards2 := range shards2 {
			if shards1.Index == shards2.Index && shards1.Shard == shards2.Shard {
				return true
			}
		}
	}
	return false
}
