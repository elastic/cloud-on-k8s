// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// Delete runs through a list of potential candidates and select the ones that can be deleted.
// Do not run this function unless driver expectations are met.
func (ctx *upgradeCtx) Delete() ([]corev1.Pod, error) {
	// Update the status with the list of Pods to be maybe upgraded here.
	ctx.reconcileState.RecordNodesToBeUpgraded(k8s.PodNames(ctx.podsToUpgrade))
	if len(ctx.podsToUpgrade) == 0 {
		// We still want to ensure that predicates in the status are cleared.
		ctx.reconcileState.RecordPredicatesResult(map[string]string{})
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
		ctx.parentCtx,
		ctx.ES,
		ctx.resourcesList,
		ctx.esState,
		ctx.shardLister,
		ctx.healthyPods,
		ctx.podsToUpgrade,
		ctx.expectedMasters,
		ctx.currentPods,
	)
	log.V(1).Info("Applying predicates",
		"maxUnavailableReached", maxUnavailableReached,
		"allowedDeletions", allowedDeletions,
	)
	podsToDelete, err := applyPredicates(predicateContext, candidates, maxUnavailableReached, allowedDeletions, ctx.reconcileState)
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

	if err := ctx.prepareClusterForNodeRestart(podsToDelete); err != nil {
		return nil, err
	}
	// TODO: If master is changed into a data node (or the opposite) it must be excluded or we should update m_m_n
	var deletedPods []corev1.Pod //nolint:prealloc
	for _, podToDelete := range podsToDelete {
		if err := ctx.handleMasterScaleChange(podToDelete); err != nil {
			return deletedPods, err
		}
		if readyToDelete, err := ctx.readyToDelete(podToDelete); err != nil || !readyToDelete {
			return deletedPods, err
		}

		if err := deletePod(ctx.client, ctx.ES, podToDelete, ctx.expectations, ctx.reconcileState, "Deleting pod for rolling upgrade"); err != nil {
			return deletedPods, err
		}
		deletedPods = append(deletedPods, podToDelete)
	}
	return deletedPods, nil
}

// DeleteAll unconditionally deletes all upgradeable Pods after calling the node shutdown API and accounting for quorum
// changes on older versions of Elasticsearch as applicable.
func (ctx *upgradeCtx) DeleteAll() ([]corev1.Pod, error) {
	if len(ctx.podsToUpgrade) == 0 {
		return nil, nil
	}

	if err := ctx.prepareClusterForNodeRestart(ctx.podsToUpgrade); err != nil {
		return nil, err
	}

	var nonReadyPods []string
	for _, podToDelete := range ctx.podsToUpgrade {
		if err := ctx.handleMasterScaleChange(podToDelete); err != nil {
			return nil, err
		}
		// do not delete any Pods if at least one is not ready for deletion
		readyToDelete, err := ctx.readyToDelete(podToDelete)
		if err != nil {
			return nil, err
		}
		if !readyToDelete {
			nonReadyPods = append(nonReadyPods, podToDelete.Name)
		}
	}

	if len(nonReadyPods) > 0 {
		log.Info("Not all Pods are ready for a full cluster upgrade", "pods", nonReadyPods, "namespace", ctx.ES.Namespace, "es_name", ctx.ES.Name)
		ctx.reconcileState.RecordNodesToBeUpgradedWithMessage(k8s.PodNames(ctx.podsToUpgrade), "Not all Pods are ready for a full cluster upgrade")
		return nil, nil
	}

	var deletedPods []corev1.Pod //nolint:prealloc
	for _, podToDelete := range ctx.podsToUpgrade {
		if err := deletePod(ctx.client, ctx.ES, podToDelete, ctx.expectations, ctx.reconcileState, "Deleting Pod for full cluster upgrade"); err != nil {
			// an error during deletion violates the "delete all or nothing" invariant but there is no way around it
			return deletedPods, err
		}
		deletedPods = append(deletedPods, podToDelete)
	}
	return deletedPods, nil
}

// getAllowedDeletions returns the number of deletions that can be done and if maxUnavailable has been reached.
func (ctx *upgradeCtx) getAllowedDeletions() (int, bool) {
	// Check if we are not over disruption budget
	// Upscale is done, we should have the required number of Pods
	actualPods := ctx.statefulSets.PodNames()
	unhealthyPods := len(actualPods) - len(ctx.healthyPods)

	maxUnavailable := ctx.ES.Spec.UpdateStrategy.ChangeBudget.GetMaxUnavailableOrDefault()
	if maxUnavailable == nil {
		// maxUnavailable is unbounded, we allow removing all pods
		return len(actualPods), false
	}

	allowedDeletions := int(*maxUnavailable) - unhealthyPods
	// If maxUnavailable is reached the deletion driver still allows one unhealthy Pod to be restarted.
	maxUnavailableReached := allowedDeletions <= 0
	return allowedDeletions, maxUnavailableReached
}

// sortCandidates is the default sort function, masters have lower priority as
// we want to update the data nodes first. After that pods are sorted by stateful set name
// then reverse ordinal order
// TODO: Add some priority to unhealthy (bootlooping) Pods
func sortCandidates(allPods []corev1.Pod) {
	sort.Slice(allPods, func(i, j int) bool {
		pod1 := allPods[i]
		pod2 := allPods[j]
		// check if either is a master node. masters come after all other roles
		if label.IsMasterNode(pod1) && !label.IsMasterNode(pod2) {
			return false
		}
		if !label.IsMasterNode(pod1) && label.IsMasterNode(pod2) {
			return true
		}
		// neither or both are masters, use the reverse name function
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
	})
}

// handleMasterScaleChange handles Zen updates when a type change results in the addition or the removal of a master:
// In case of a master scale down it shares the same logic that a "traditional" scale down:
// * We proactively set m_m_n to the value of 1 if there are 2 Zen1 masters left
// * We exclude the master for Zen2
// In case of a master scale up there's nothing else to do:
// * If there are Zen1 nodes m_m_n is updated prior the update of the StatefulSet in HandleUpscaleAndSpecChanges
// * Because of the design of Zen2 there's nothing else to do for it.
func (ctx *upgradeCtx) handleMasterScaleChange(pod corev1.Pod) error {
	masterScaleDown := label.IsMasterNode(pod) && !stringsutil.StringInSlice(pod.Name, ctx.expectedMasters)
	if masterScaleDown {
		if err := updateZenSettingsForDownscale(
			ctx.parentCtx,
			ctx.client,
			ctx.esClient,
			ctx.ES,
			ctx.reconcileState,
			ctx.statefulSets,
			pod.Name,
		); err != nil {
			return err
		}
	}
	return nil
}

func deletePod(
	k8sClient k8s.Client,
	es esv1.Elasticsearch,
	pod corev1.Pod,
	expectations *expectations.Expectations,
	reconcileState *reconcile.State,
	msg string,
) error {
	log.Info(msg, "es_name", es.Name, "namespace", es.Namespace, "pod_name", pod.Name, "pod_uid", pod.UID)
	// The name of the Pod we want to delete is not enough as it may have been already deleted/recreated.
	// The uid of the Pod we want to delete is used as a precondition to check that we actually delete the right one.
	// We also check the version of the Pod resource, to make sure its status is the current one and we're not deleting
	// eg. a Pending Pod that is not Pending anymore.
	opt := client.Preconditions{
		UID:             &pod.UID,
		ResourceVersion: &pod.ResourceVersion,
	}
	err := k8sClient.Delete(context.Background(), &pod, opt)
	if err != nil {
		return err
	}
	// expect the pod to not be there in the cache at next reconciliation
	expectations.ExpectDeletion(pod)
	// Update status
	reconcileState.RecordDeletedNode(pod.Name, msg)
	return nil
}

// runPredicates runs all the predicates on a given Pod. Result is non nil if a predicate has failed.
// The second error is non nil if one of the predicate encountered an internal error.
func runPredicates(
	ctx PredicateContext,
	candidate corev1.Pod,
	deletedPods []corev1.Pod,
	maxUnavailableReached bool,
) (*failedPredicate, error) {
	disabledPredicates := ctx.es.DisabledPredicates()
	for _, predicate := range predicates {
		canDelete, err := predicate.fn(ctx, candidate, deletedPods, maxUnavailableReached)
		if err != nil {
			return nil, err
		}
		if !canDelete {
			// if this specific predicate name is disabled by the disable predicate annotation
			// "eck.k8s.elastic.co/disable-upgrade-predicates", then ignore this predicate,
			// and continue processing the remaining predicates.
			if disabledPredicates.Has(predicate.name) || disabledPredicates.Has("*") {
				log.Info("Warning: disabling upgrade predicate because of annotation", "predicate", predicate.name, "namespace", ctx.es.Namespace, "es_name", ctx.es.Name)
				continue
			}
			// Skip this Pod, it can't be deleted for the moment
			return &failedPredicate{
				pod:       candidate.Name,
				predicate: predicate.name,
			}, nil
		}
	}
	// All predicates passed!
	return nil, nil
}
