// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Delete runs through a list of potential candidates and select the ones that can be deleted.
// Do not run this function unless driver expectations are met.
func (ctx *rollingUpgradeCtx) Delete() ([]corev1.Pod, error) {
	if len(ctx.podsToUpgrade) == 0 {
		return nil, nil
	}

	// Check if we are not over disruption budget
	// Upscale is done, we should have the required number of Pods
	expectedPods := ctx.statefulSets.PodNames()
	unhealthyPods := len(expectedPods) - len(ctx.healthyPods)
	maxUnavailable := 1
	// TODO: use GroupingDefinition
	if ctx.ES.Spec.UpdateStrategy.ChangeBudget != nil {
		maxUnavailable = ctx.ES.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable
	}
	allowedDeletions := maxUnavailable - unhealthyPods
	// If maxUnavailable is reached the deletion driver still allows one unhealthy Pod to be restarted.
	// TODO: Should we make the difference between MaxUnavailable and MaxConcurrentRestarting ?
	maxUnavailableReached := allowedDeletions <= 0

	// Step 2. Sort the Pods to get the ones with the higher priority
	candidates := make([]corev1.Pod, len(ctx.podsToUpgrade)) // work on a copy in order to have no side effect
	copy(candidates, ctx.podsToUpgrade)
	sortCandidates(candidates)

	// Step 3: Apply predicates
	predicateContext := NewPredicateContext(
		ctx.esState,
		ctx.healthyPods,
		ctx.podsToUpgrade,
		ctx.expectedMasters,
	)
	log.V(1).Info("applying predicates",
		"maxUnavailableReached", maxUnavailableReached,
		"allowedDeletions", allowedDeletions,
	)
	podsToDelete, err := applyPredicates(predicateContext, candidates, maxUnavailableReached, allowedDeletions)
	if err != nil {
		return podsToDelete, err
	}

	if len(podsToDelete) == 0 {
		log.Info("no pod deleted", "es_name", ctx.ES.Name, "es_namespace", ctx.ES.Namespace)
		return podsToDelete, nil
	}

	// Disable shard allocation
	if err := ctx.prepareClusterForNodeRestart(ctx.esClient, ctx.esState); err != nil {
		return podsToDelete, err
	}
	// TODO: If master is changed into a data node (or the opposite) it must be excluded or we should update m_m_n
	deletedPods := []corev1.Pod{}
	for _, podToDelete := range podsToDelete {
		ctx.expectations.ExpectDeletion(podToDelete)
		err := ctx.delete(&podToDelete)
		if err != nil {
			ctx.expectations.CancelExpectedDeletion(podToDelete)
			return deletedPods, err
		}
		deletedPods = append(deletedPods, podToDelete)
	}
	return deletedPods, nil
}

func applyPredicates(ctx PredicateContext, candidates []corev1.Pod, maxUnavailableReached bool, allowedDeletions int) (deletedPods []corev1.Pod, err error) {
	for _, candidate := range candidates {
		if ok, err := runPredicates(ctx, candidate, deletedPods, maxUnavailableReached); err != nil {
			return deletedPods, err
		} else if ok {
			candidate := candidate
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

func (ctx *rollingUpgradeCtx) delete(pod *corev1.Pod) error {
	uid := pod.UID
	log.Info("delete pod", "es_name", ctx.ES.Name, "namespace", ctx.ES.Namespace, "pod_name", pod.Name, "pod_uid", pod.UID)
	return ctx.client.Delete(pod, func(options *client.DeleteOptions) {
		if options.Preconditions == nil {
			options.Preconditions = &metav1.Preconditions{}
		}
		options.Preconditions.UID = &uid
	})
}

func runPredicates(
	ctx PredicateContext,
	candidate corev1.Pod,
	deletedPods []corev1.Pod,
	maxUnavailableReached bool,
) (bool, error) {
	for _, predicate := range predicates {
		canDelete, err := predicate.fn(ctx, candidate, deletedPods, maxUnavailableReached)
		if err != nil {
			return false, err
		}
		if !canDelete {
			log.V(1).Info("predicate failed", "pod_name", candidate.Name, "predicate_name", predicate.name)
			// Skip this Pod, it can't be deleted for the moment
			return false, nil
		}
	}
	// All predicates passed!
	return true, nil
}

func (ctx *rollingUpgradeCtx) prepareClusterForNodeRestart(esClient esclient.Client, esState ESState) error {
	// Disable shard allocations to avoid shards moving around while the node is temporarily down
	shardsAllocationEnabled, err := esState.ShardAllocationsEnabled()
	if err != nil {
		return err
	}
	if shardsAllocationEnabled {
		log.Info("Disabling shards allocation", "es_name", ctx.ES.Name, "namespace", ctx.ES.Namespace)
		if err := disableShardsAllocation(esClient); err != nil {
			return err
		}
	}

	// Request a sync flush to optimize indices recovery when the node restarts.
	if err := doSyncFlush(esClient); err != nil {
		return err
	}

	// TODO: halt ML jobs on that node
	return nil
}
