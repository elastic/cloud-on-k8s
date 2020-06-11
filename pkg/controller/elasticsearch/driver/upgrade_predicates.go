// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
)

type PredicateContext struct {
	es                     esv1.Elasticsearch
	masterNodesNames       []string
	actualMasters          []corev1.Pod
	healthyPods            map[string]corev1.Pod
	toUpdate               []corev1.Pod
	esState                ESState
	shardLister            client.ShardLister
	masterUpdateInProgress bool
	ctx                    context.Context
}

// Predicate is a function that indicates if a Pod can be deleted (or not).
type Predicate struct {
	name string
	fn   func(context PredicateContext, candidate corev1.Pod, deletedPods []corev1.Pod, maxUnavailableReached bool) (bool, error)
}

type failedPredicate struct {
	pod       string
	predicate string
}

type failedPredicates []failedPredicate

// groupByPredicates groups by predicates the pods that can't be upgraded.
func groupByPredicates(fp failedPredicates) map[string][]string {
	podsByPredicates := make(map[string][]string)
	for _, failedPredicate := range fp {
		pods := podsByPredicates[failedPredicate.predicate]
		pods = append(pods, failedPredicate.pod)
		podsByPredicates[failedPredicate.predicate] = pods
	}
	return podsByPredicates
}

func NewPredicateContext(
	ctx context.Context,
	es esv1.Elasticsearch,
	state ESState,
	shardLister client.ShardLister,
	healthyPods map[string]corev1.Pod,
	podsToUpgrade []corev1.Pod,
	masterNodesNames []string,
	actualMasters []corev1.Pod,
) PredicateContext {
	return PredicateContext{
		es:               es,
		masterNodesNames: masterNodesNames,
		actualMasters:    actualMasters,
		healthyPods:      healthyPods,
		toUpdate:         podsToUpgrade,
		esState:          state,
		shardLister:      shardLister,
		ctx:              ctx,
	}
}

func applyPredicates(ctx PredicateContext, candidates []corev1.Pod, maxUnavailableReached bool, allowedDeletions int) (deletedPods []corev1.Pod, err error) {
	var failedPredicates failedPredicates

Loop:
	for _, candidate := range candidates {
		switch predicateErr, err := runPredicates(ctx, candidate, deletedPods, maxUnavailableReached); {
		case err != nil:
			return deletedPods, err
		case predicateErr != nil:
			// A predicate has failed on this Pod
			failedPredicates = append(failedPredicates, *predicateErr)
		default:
			candidate := candidate
			if label.IsMasterNode(candidate) || willBecomeMasterNode(candidate.Name, ctx.masterNodesNames) {
				// It is a mutation on an already existing or future master.
				ctx.masterUpdateInProgress = true
			}
			// Remove from healthy nodes if it was there
			delete(ctx.healthyPods, candidate.Name)
			// Append to the deletedPods list
			deletedPods = append(deletedPods, candidate)
			allowedDeletions--
			if allowedDeletions <= 0 {
				break Loop
			}
		}
	}

	// If some predicates have failed print a summary of the failures to help
	// the user to understand why.
	if len(failedPredicates) > 0 {
		log.Info(
			"Cannot restart some nodes for upgrade at this time",
			"namespace", ctx.es.Namespace,
			"es_name", ctx.es.Name,
			"failed_predicates", groupByPredicates(failedPredicates))
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
		// If health is not Green or Yellow only allow unhealthy Pods to be restarted.
		// This is intended to unlock some situations where the cluster is not green and
		// a Pod has to be restarted a second time.
		name: "only_restart_healthy_node_if_green_or_yellow",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			// Cluster health is retrieved only once from the cluster.
			// We rely on "shard conflict" predicate to avoid to delete two ES nodes that share some shards.
			health, err := context.esState.Health()
			if err != nil {
				return false, err
			}
			if health.Status == esv1.ElasticsearchGreenHealth || health.Status == esv1.ElasticsearchYellowHealth {
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
		// During a rolling upgrade, primary shards assigned to a node running a new version cannot have their
		// replicas assigned to a node with the old version. Therefore we must allow some Pods to be restarted
		// even if cluster health is Yellow so the replicas can be assigned.
		// This predicate checks that the following conditions are met for a candidate:
		// * A cluster upgrade is in progress and the candidate version is not up to date
		// * All primaries are assigned, only replicas are actually not assigned
		// * There are no initializing or relocating shards
		// See https://github.com/elastic/cloud-on-k8s/issues/1643
		name: "if_yellow_only_restart_upgrading_nodes_with_unassigned_replicas",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			health, err := context.esState.Health()
			if err != nil {
				return false, err
			}
			_, healthyNode := context.healthyPods[candidate.Name]
			if health.Status != esv1.ElasticsearchYellowHealth || !healthyNode {
				// This predicate is only relevant on healthy node if cluster health is yellow
				return true, nil
			}
			version := candidate.Labels[label.VersionLabelName]
			if version == context.es.Spec.Version {
				// Restart in yellow state is only allowed during version upgrade
				return false, nil
			}
			// This candidate needs a version upgrade, check if the primaries are assigned and shards are not moving or
			// initializing
			return isSafeToRoll(health), nil
		},
	},
	{
		// We may need to delete nodes in a yellow cluster, but not if they contain the only replica
		// of a shard since it would make the cluster go red.
		name: "require_started_replica",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			maxUnavailableReached bool,
		) (b bool, e error) {
			allShards, err := context.shardLister.GetShards(context.ctx)
			if err != nil {
				return false, err
			}
			// We maintain two data structures to record:
			// * The total number of replicas for a shard
			// * How many of them are STARTED
			startedReplicas := make(map[string]int)
			replicas := make(map[string]int)
			for _, shard := range allShards {
				if shard.NodeName == candidate.Name {
					continue
				}
				shardKey := shard.Key()
				replicas[shardKey]++
				if shard.State == client.STARTED {
					startedReplicas[shardKey]++
				}
			}

			// Do not delete a node with a Primary if there is not at least one STARTED replica
			shardsByNode := allShards.GetShardsByNode()
			shardsOnCandidate := shardsByNode[candidate.Name]
			for _, shard := range shardsOnCandidate {
				if !shard.IsPrimary() {
					continue
				}
				shardKey := shard.Key()
				numReplicas := replicas[shardKey]
				assignedReplica := startedReplicas[shardKey]
				// We accept here that there will be some unavailability if an index is configured with zero replicas
				if numReplicas > 0 && assignedReplica == 0 {
					// If this node is deleted there will be no more shards available
					return false, nil
				}
			}
			return true, nil
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
				if willBecomeMasterNode(candidate.Name, context.masterNodesNames) {
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
			if !willBecomeMasterNode(candidate.Name, context.masterNodesNames) {
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

			shards, err := context.shardLister.GetShards(context.ctx)
			if err != nil {
				return true, err
			}
			shardsByNode := shards.GetShardsByNode()
			shardsOnCandidate, ok := shardsByNode[candidate.Name]
			if !ok {
				// No shards on this node
				return true, nil
			}

			for _, deletedPod := range deletedPods {
				shardsOnDeletedPod, ok := shardsByNode[deletedPod.Name]
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

func willBecomeMasterNode(name string, masters []string) bool {
	return stringsutil.StringInSlice(name, masters)
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

// IsSafeToRoll indicates that a rolling update can continue with the next node if
// - no relocating or initializing shards or shards being fetched
// - all primaries allocated
// only reliable if Status result was created with wait_for_events=languid
// so that there are no pending initialisations in the task queue
func isSafeToRoll(health client.Health) bool {
	return !health.TimedOut && // make sure request did not time out (i.e. no pending events)
		health.Status != esv1.ElasticsearchRedHealth && // all primaries allocated
		health.NumberOfInFlightFetch == 0 && // no shards being fetched
		health.InitializingShards == 0 && // no shards initializing
		health.RelocatingShards == 0 // no shards relocating
}
