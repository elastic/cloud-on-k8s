// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"sort"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
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

	// Get allowed deletions and check if maxUnavailable has been reached.
	allowedDeletions, maxUnavailableReached := ctx.getAllowedDeletions()

	// Step 1. Sort the Pods to get the ones with the higher priority
	candidates := make([]corev1.Pod, len(ctx.podsToUpgrade)) // work on a copy in order to have no side effect
	copy(candidates, ctx.podsToUpgrade)
	sortCandidates(candidates)

	// Step 2: Apply predicates
	predicateContext := NewPredicateContext(
		ctx.esState,
		ctx.healthyPods,
		ctx.podsToUpgrade,
		ctx.expectedMasters,
	)
	log.V(1).Info("Applying predicates",
		"maxUnavailableReached", maxUnavailableReached,
		"allowedDeletions", allowedDeletions,
	)
	podsToDelete, err := applyPredicates(predicateContext, candidates, maxUnavailableReached, allowedDeletions)
	if err != nil {
		return podsToDelete, err
	}

	if len(podsToDelete) == 0 {
		log.V(1).Info(
			"No pod deleted during rolling upgrade",
			"es_name", ctx.ES.Name,
			"namespace", ctx.ES.Namespace,
		)
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

// getAllowedDeletions returns the number of deletions that can be done and if maxUnavailable has been reached.
func (ctx *rollingUpgradeCtx) getAllowedDeletions() (int, bool) {
	// Check if we are not over disruption budget
	// Upscale is done, we should have the required number of Pods
	expectedPods := ctx.statefulSets.PodNames()
	unhealthyPods := len(expectedPods) - len(ctx.healthyPods)
	maxUnavailable := 1
	if ctx.ES.Spec.UpdateStrategy.ChangeBudget != nil {
		maxUnavailable = ctx.ES.Spec.UpdateStrategy.ChangeBudget.MaxUnavailable
	}
	allowedDeletions := maxUnavailable - unhealthyPods
	// If maxUnavailable is reached the deletion driver still allows one unhealthy Pod to be restarted.
	maxUnavailableReached := allowedDeletions <= 0
	return allowedDeletions, maxUnavailableReached
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

func (ctx *rollingUpgradeCtx) delete(pod *corev1.Pod) error {
	uid := pod.UID
	log.Info("Deleting pod for rolling upgrade", "es_name", ctx.ES.Name, "namespace", ctx.ES.Namespace, "pod_name", pod.Name, "pod_uid", pod.UID)
	return ctx.client.Delete(pod, func(options *client.DeleteOptions) {
		if options.Preconditions == nil {
			options.Preconditions = &metav1.Preconditions{}
		}
		// The name of the Pod we want to delete is not enough as it may have been already deleted/recreated.
		// The uid of the Pod we want to delete is used as a precondition to check that we actually delete the right one.
		// If not it means that we are running with a stale cache.
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
			log.V(1).Info("Predicate failed", "pod_name", candidate.Name, "predicate_name", predicate.name)
			// Skip this Pod, it can't be deleted for the moment
			return false, nil
		}
	}
	// All predicates passed!
	return true, nil
}
