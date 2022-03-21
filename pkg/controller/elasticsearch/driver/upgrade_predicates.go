// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// getNodeSettings returns the node settings for a given Pod.
func getNodeSettings(
	version version.Version,
	resourcesList nodespec.ResourcesList,
	pod corev1.Pod,
) (esv1.ElasticsearchSettings, error) {
	// Get the expected configuration
	statefulSetName, _, err := sset.StatefulSetName(pod.Name)
	if err != nil {
		return esv1.ElasticsearchSettings{}, err
	}
	resources, err := resourcesList.ForStatefulSet(statefulSetName)
	if err != nil {
		return esv1.ElasticsearchSettings{}, err
	}
	nodeCfg, err := resources.Config.Unpack(version)
	if err != nil {
		return esv1.ElasticsearchSettings{}, err
	}
	return nodeCfg, nil
}

// getNodesSettings returns the node settings for a given list of Pods.
func getNodesSettings(
	version version.Version,
	resourcesList nodespec.ResourcesList,
	pods ...corev1.Pod,
) ([]esv1.ElasticsearchSettings, error) {
	rolesList := make([]esv1.ElasticsearchSettings, len(pods))
	for i := range pods {
		roles, err := getNodeSettings(version, resourcesList, pods[i])
		if err != nil {
			return nil, err
		}
		rolesList[i] = roles
	}
	return rolesList, nil
}

// hasDependencyInOthers returns true if, for a given node, at least one other node in a slice can be considered as a
// strong dependency and must be upgraded first. A strong dependency is a unidirectional dependency, if a circular
// dependency exists between two nodes the dependency is not considered as a strong one.
func hasDependencyInOthers(node esv1.ElasticsearchSettings, others []esv1.ElasticsearchSettings) bool {
	if !node.Node.CanContainData() {
		// node has no tier which requires upgrade prioritization.
		return false
	}
	for _, other := range others {
		if !other.Node.CanContainData() {
			// this other node has no tier which requires upgrade prioritization.
			continue
		}
		if node.Node.DependsOn(other.Node) && !other.Node.DependsOn(node.Node) {
			// candidate has this other node as a strict dependency
			return true
		}
	}
	// no dependency or roles are overlapping, we still allow the upgrade
	return false
}

// PredicateContext is the set of fields used while determining what set of pods
// can be upgraded when performing a rolling upgrade on an Elasticsearch cluster.
type PredicateContext struct {
	es esv1.Elasticsearch
	// expected resources (sset, service, config) by StatefulSet
	resourcesList            nodespec.ResourcesList
	expectedMasterNodesNames []string
	// healthy Pods are "running" as per k8s API and have joined the ES cluster
	healthyPods map[string]corev1.Pod
	// Pods based on outdated spec
	toUpdate               []corev1.Pod
	esState                ESState
	shardLister            client.ShardLister
	masterUpdateInProgress bool
	ctx                    context.Context
	// all Pods for the existing StatefulSets from k8s API
	currentPods []corev1.Pod
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

type failedPredicates map[string]string

// groupByPredicates groups by predicates the pods that can't be upgraded.
func groupByPredicates(fp failedPredicates) map[string][]string {
	podsByPredicates := make(map[string][]string)
	for pod, predicate := range fp {
		pods := podsByPredicates[predicate]
		pods = append(pods, pod)
		podsByPredicates[predicate] = pods
	}
	// Sort pods for stable comparison
	for _, pods := range podsByPredicates {
		sort.Strings(pods)
	}
	return podsByPredicates
}

// NewPredicateContext returns a new predicate context for use when
// processing an Elasticsearch rolling upgrade.
func NewPredicateContext(
	ctx context.Context,
	es esv1.Elasticsearch,
	resourcesList nodespec.ResourcesList,
	state ESState,
	shardLister client.ShardLister,
	healthyPods map[string]corev1.Pod,
	podsToUpgrade []corev1.Pod,
	masterNodesNames []string,
	currentPods []corev1.Pod,
) PredicateContext {
	return PredicateContext{
		es:                       es,
		resourcesList:            resourcesList,
		expectedMasterNodesNames: masterNodesNames,
		healthyPods:              healthyPods,
		toUpdate:                 podsToUpgrade,
		esState:                  state,
		shardLister:              shardLister,
		ctx:                      ctx,
		currentPods:              currentPods,
	}
}

func applyPredicates(
	ctx PredicateContext,
	candidates []corev1.Pod,
	maxUnavailableReached bool,
	allowedDeletions int,
	reconcileState *reconcile.State,
) (deletedPods []corev1.Pod, err error) {
	failedPredicates := make(failedPredicates)

Loop:
	for _, candidate := range candidates {
		switch predicateErr, err := runPredicates(ctx, candidate, deletedPods, maxUnavailableReached); {
		case err != nil:
			return deletedPods, err
		case predicateErr != nil:
			// A predicate has failed on this Pod
			failedPredicates[predicateErr.pod] = predicateErr.predicate
		default:
			candidate := candidate
			if label.IsMasterNode(candidate) || willBecomeMasterNode(candidate.Name, ctx.expectedMasterNodesNames) {
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
	groupByPredicates := groupByPredicates(failedPredicates)
	if len(failedPredicates) > 0 {
		log.Info(
			"Cannot restart some nodes for upgrade at this time",
			"namespace", ctx.es.Namespace,
			"es_name", ctx.es.Name,
			"failed_predicates", groupByPredicates)
	}
	// Also report in the status
	reconcileState.RecordPredicatesResult(failedPredicates)
	return deletedPods, nil
}

var predicates = [...]Predicate{
	{
		name: "data_tier_with_higher_priority_must_be_upgraded_first",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			deletedPods []corev1.Pod,
			_ bool,
		) (b bool, e error) {
			if candidate.Labels[label.VersionLabelName] == context.es.Spec.Version {
				// This predicate is only relevant during version upgrade.
				return true, nil
			}

			currentVersion, err := label.ExtractVersion(candidate.Labels)
			if err != nil {
				return false, err
			}
			if currentVersion.LT(version.From(7, 10, 0)) {
				// This predicate is only valid for an Elasticsearch node handling data tiers.
				return true, nil
			}

			expectedVersion, err := version.Parse(context.es.Spec.Version)
			if err != nil {
				return false, err
			}
			// Get roles for the candidate.
			candidateRoles, err := getNodeSettings(expectedVersion, context.resourcesList, candidate)
			if err != nil {
				return false, err
			}

			// Get all the roles from the Pods to be upgraded, including the ones already scheduled for an upgrade:
			// the intent is to upgrade all the nodes with a same priority before moving on to a tier with a lower priority.
			allPods := append(context.toUpdate, deletedPods...)
			otherRoles, err := getNodesSettings(expectedVersion, context.resourcesList, allPods...)
			if err != nil {
				return false, err
			}

			if hasDependencyInOthers(candidateRoles, otherRoles) {
				return false, err
			}
			return true, nil
		},
	},
	{
		// If MaxUnavailable is reached, only allow unhealthy Pods to be deleted.
		// This is to prevent a situation where MaxUnavailable is reached and we
		// can't make progress even if the user has updated the spec.
		name: "do_not_restart_healthy_node_if_MaxUnavailable_reached",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			_ []corev1.Pod,
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
			_ bool,
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
			_ []corev1.Pod,
			_ bool,
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
			_ []corev1.Pod,
			_ bool,
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
			if len(context.currentPods) == 1 {
				// If the cluster is a single node cluster, allow restart even if there is no version difference
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
			_ []corev1.Pod,
			_ bool,
		) (b bool, e error) {
			health, err := context.esState.Health()
			if err != nil {
				return false, err
			}
			_, healthyNode := context.healthyPods[candidate.Name]
			if len(context.currentPods) == 1 && health.Status == esv1.ElasticsearchYellowHealth && healthyNode {
				// If the cluster is a healthy single node cluster, replicas can not be started, allow the upgrade
				return true, nil
			}
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
			_ []corev1.Pod,
			_ bool,
		) (b bool, e error) {

			// If candidate is not a master then we just check if it will become a master
			// In this case we account for a master creation as we want to avoid creating more
			// than one master at a time.
			if !label.IsMasterNode(candidate) {
				if willBecomeMasterNode(candidate.Name, context.expectedMasterNodesNames) {
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
			if !willBecomeMasterNode(candidate.Name, context.expectedMasterNodesNames) {
				// We still need to ensure that others masters are healthy
				for _, actualMaster := range label.FilterMasterNodePods(context.currentPods) {
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
			expectedMasters := len(context.expectedMasterNodesNames)
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
		// Force an upgrade of all the master-ineligible nodes before upgrading the last master-eligible
		name: "do_not_delete_last_master_if_all_master_ineligible_nodes_are_not_upgraded",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			_ []corev1.Pod,
			_ bool,
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
			// This is the last master, check if all master-ineligible nodes are up-to-date
			for _, pod := range context.toUpdate {
				if candidate.Name == pod.Name {
					continue
				}
				if !label.IsMasterNode(pod) {
					// There's still some master-ineligible nodes to update
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
			_ bool,
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
	{
		name: "do_not_delete_all_members_of_a_tier",
		fn: func(
			context PredicateContext,
			candidate corev1.Pod,
			_ []corev1.Pod,
			_ bool,
		) (bool, error) {
			if _, exists := context.healthyPods[candidate.Name]; !exists {
				// there is no point in keeping an unhealthy Pod as it does not contribute to tier availability
				return true, nil
			}

			healthyPodRoleHisto := map[common.TrueFalseLabel]int{}
			currentPodRoleHisto := map[common.TrueFalseLabel]int{}
			// look at the current pods because we want to prevent taking away from current capacity for a tier
			// not future capacity as per the expected Pod definitions expressed in the resourcesList
			for _, pod := range context.currentPods {
				// ignore voting_only and master we are handling those in dedicated predicates
				forEachNonMasterRole(pod, func(role common.TrueFalseLabel) {
					currentPodRoleHisto[role]++
				})
			}

			// look at the healthy Pods excluding the candidate (which is part of this list)
			// this allows us to figure out what would be the remaining Pods assuming we remove the candidate
			for _, pod := range context.healthyPods {
				if pod.Name == candidate.Name {
					continue
				}
				forEachNonMasterRole(pod, func(role common.TrueFalseLabel) {
					healthyPodRoleHisto[role]++
				})
			}

			for _, role := range label.NonMasterRoles {
				if role.HasValue(true, candidate.Labels) {
					healthy := healthyPodRoleHisto[role]
					current := currentPodRoleHisto[role]
					if current == 1 {
						// only Pod with this role: OK to delete
						continue
					}
					if healthy <= 0 {
						log.V(1).Info(
							"Delaying upgrade for Pod to ensure tier availability",
							"node_role", role,
							"namespace", candidate.Namespace,
							"candidate", candidate.Name,
							"healthy_pods_with_role", healthy,
						)
						return false, nil
					}
				}
			}
			return true, nil
		},
	},
}

func forEachNonMasterRole(pod corev1.Pod, f func(falseLabel common.TrueFalseLabel)) {
	for _, role := range label.NonMasterRoles {
		if role.HasValue(true, pod.Labels) {
			f(role)
		}
	}
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
