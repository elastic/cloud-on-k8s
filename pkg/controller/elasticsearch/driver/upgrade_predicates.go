// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
)

type PredicateContext struct {
	masterNodesNames       []string
	actualMasters          []corev1.Pod
	healthyPods            map[string]corev1.Pod
	toUpdate               []corev1.Pod
	esState                ESState
	masterUpdateInProgress bool
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
	actualMasters []corev1.Pod,
) PredicateContext {
	return PredicateContext{
		masterNodesNames: masterNodesNames,
		actualMasters:    actualMasters,
		healthyPods:      healthyPods,
		toUpdate:         podsToUpgrade,
		esState:          state,
	}
}

func applyPredicates(ctx PredicateContext, candidates []corev1.Pod, maxUnavailableReached bool, allowedDeletions int) (deletedPods []corev1.Pod, err error) {
	for _, candidate := range candidates {
		if ok, err := runPredicates(ctx, candidate, deletedPods, maxUnavailableReached); err != nil {
			return deletedPods, err
		} else if ok {
			candidate := candidate
			if label.IsMasterNode(candidate) || stringsutil.StringInSlice(candidate.Name, ctx.masterNodesNames) {
				// It is a mutation on an already existing or future master.
				ctx.masterUpdateInProgress = true
			}
			// Remove from healthy nodes if it was there
			delete(ctx.healthyPods, candidate.Name)
			// Append to the deletedPods list
			deletedPods = append(deletedPods, candidate)
			allowedDeletions--
			if allowedDeletions <= 0 {
				break
			}
		}
	}
	return deletedPods, nil
}

var predicates = [...]Predicate{
	{
		// If MaxUnavailable is reached, only allow unhealthy Pods to be deleted.
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
			// We rely on "shard conflict" predicate to avoid to delete two ES nodes that share some shards.
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

			// If candidate is not a master then we just check if it will become a master
			// In this case we account for a master creation as we want to avoid creating more
			// than one master at a time.
			if !label.IsMasterNode(candidate) {
				if stringsutil.StringInSlice(candidate.Name, context.masterNodesNames) {
					// TODO: if this one is becoming a master we probably want to check here if the candidate is healthy,
					//  reason is that adding a new master that can't start could break the quorum (at least for Zen1).
					return !context.masterUpdateInProgress, nil
				}
				// It is just a data node and it will not become a master: we don't care
				return true, nil
			}

			// There is a current master scheduled for deletion
			if context.masterUpdateInProgress {
				return false, nil
			}

			// If candidate is already a master and is not healthy we want to give it a chance to restart anyway
			// even if it is leaving the control plane.
			_, healthy := context.healthyPods[candidate.Name]
			if !healthy {
				return true, nil
			}

			// If Pod is not an expected master it means that we are downscaling the masters
			// by changing the type of the node.
			// In this case we still check that other masters are healthy to avoid degrading the situation.
			if !stringsutil.StringInSlice(candidate.Name, context.masterNodesNames) {
				// We still need to ensure that others masters are healthy
				for _, actualMaster := range context.actualMasters {
					_, healthyMaster := context.healthyPods[actualMaster.Name]
					if !healthyMaster {
						log.V(1).Info(
							"Can't permanently remove a master in a rolling upgrade if there is an other unhealthy master",
							"namespace", candidate.Namespace,
							"candidate", candidate.Name,
							"unhealthy", actualMaster.Name,
						)
						return false, nil
					}
				}
				return true, nil
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
			// We are relying here on the expectations and on the checks above that give us
			// the guarantee that there is no upscale or downscale in progress.
			// The condition to update an existing master is to have all the masters in a healthy state.
			if healthyMasters == expectedMasters {
				return true, nil
			}
			log.V(1).Info(
				"Cannot delete master for rolling upgrade",
				"expected_healthy_masters", expectedMasters,
				"actually_healthy_masters", healthyMasters,
			)
			return false, nil
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
					// There are some other masters to upgrades, allow this one to be deleted
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
