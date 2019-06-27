// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"sort"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/mutation/comparison"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	corev1 "k8s.io/api/core/v1"
)

// PodBuilder is a function that is able to create pods from a PodSpecContext,
// mostly used by the various supported versions
type PodBuilder func(ctx pod.PodSpecContext) corev1.Pod

// PodComparisonResult holds information about pod comparison result
type PodComparisonResult struct {
	IsMatch               bool
	MatchingPod           pod.PodWithConfig
	MismatchReasonsPerPod map[string][]string
	RemainingPods         pod.PodsWithConfig
}

// CalculateChanges returns Changes to perform by comparing actual pods to expected pods spec
func CalculateChanges(
	es v1alpha1.Elasticsearch,
	expectedPodSpecCtxs []pod.PodSpecContext,
	state reconcile.ResourcesState,
	podBuilder PodBuilder,
	allowPVCReuse bool,
) (Changes, error) {
	// work on copies of the arrays, on which we can safely remove elements
	expectedCopy := make([]pod.PodSpecContext, len(expectedPodSpecCtxs))
	copy(expectedCopy, expectedPodSpecCtxs)
	actualCopy := make(pod.PodsWithConfig, len(state.CurrentPods))
	copy(actualCopy, state.CurrentPods)
	deletingCopy := make(pod.PodsWithConfig, len(state.DeletingPods))
	copy(deletingCopy, state.DeletingPods)

	return mutableCalculateChanges(es, expectedCopy, actualCopy, state, podBuilder, deletingCopy, allowPVCReuse)
}

func mutableCalculateChanges(
	es v1alpha1.Elasticsearch,
	expectedPodSpecCtxs []pod.PodSpecContext,
	actualPods pod.PodsWithConfig,
	state reconcile.ResourcesState,
	podBuilder PodBuilder,
	deletingPods pod.PodsWithConfig,
	allowPVCReuse bool,
) (Changes, error) {
	changes := EmptyChanges()

	for _, expectedPodSpecCtx := range expectedPodSpecCtxs {

		// look for a matching pod in the current ones
		actualComparisonResult, err := getAndRemoveMatchingPod(es, expectedPodSpecCtx, actualPods, state)
		if err != nil {
			return changes, err
		}
		if actualComparisonResult.IsMatch {
			// matching pod already exists, keep it
			changes.ToKeep = append(changes.ToKeep, actualComparisonResult.MatchingPod)
			// one less pod to compare with
			actualPods = actualComparisonResult.RemainingPods
			continue
		}

		// look for a matching pod in the ones that are being deleted
		deletingComparisonResult, err := getAndRemoveMatchingPod(es, expectedPodSpecCtx, deletingPods, state)
		if err != nil {
			return changes, err
		}
		if deletingComparisonResult.IsMatch {
			// a matching pod is terminating, wait in order to reuse its resources
			changes.ToKeep = append(changes.ToKeep, deletingComparisonResult.MatchingPod)
			// one less pod to compare with
			deletingPods = deletingComparisonResult.RemainingPods
			continue
		}

		// no matching pod, a new one should be created
		pod := podBuilder(expectedPodSpecCtx)

		changes.ToCreate = append(changes.ToCreate, PodToCreate{
			Pod:             pod,
			PodSpecCtx:      expectedPodSpecCtx,
			MismatchReasons: actualComparisonResult.MismatchReasonsPerPod,
		})
	}
	// remaining actual pods should be deleted
	for _, p := range actualPods {
		changes.ToDelete = append(changes.ToDelete, PodToDelete{PodWithConfig: p, ReusePVC: false})
	}

	// sort changes for idempotent processing
	sort.SliceStable(changes.ToKeep, sortPodByCreationTimestampAsc(changes.ToKeep))
	sort.SliceStable(changes.ToDelete, sortPodtoDeleteByCreationTimestampAsc(changes.ToDelete))

	if allowPVCReuse {
		// try to reuse PVCs of pods to delete for pods to create
		changes = optimizeForPVCReuse(changes, state)
	}

	return changes, nil
}

func getAndRemoveMatchingPod(
	es v1alpha1.Elasticsearch,
	podSpecCtx pod.PodSpecContext,
	podsWithConfig pod.PodsWithConfig,
	state reconcile.ResourcesState,
) (PodComparisonResult, error) {
	mismatchReasonsPerPod := map[string][]string{}

	for i, podWithConfig := range podsWithConfig {
		pod := podWithConfig.Pod

		isMatch, mismatchReasons, err := comparison.PodMatchesSpec(es, podWithConfig, podSpecCtx, state)
		if err != nil {
			return PodComparisonResult{}, err
		}
		if isMatch {
			// matching pod found
			// remove it from the remaining pods
			return PodComparisonResult{
				IsMatch:               true,
				MatchingPod:           podWithConfig,
				MismatchReasonsPerPod: mismatchReasonsPerPod,
				RemainingPods:         append(podsWithConfig[:i], podsWithConfig[i+1:]...),
			}, nil
		}
		mismatchReasonsPerPod[pod.Name] = mismatchReasons
	}
	// no matching pod found
	return PodComparisonResult{
		IsMatch:               false,
		MismatchReasonsPerPod: mismatchReasonsPerPod,
		RemainingPods:         podsWithConfig,
	}, nil
}

// calculatePodsToReplace modifies changes to account for pods to delete
// that can be replaced by pods to create while reusing PVCs.
// When we find a delete/create match, we tag the pod to delete to preserve its PVC on deletion.
// The corresponding pod to create is removed from changes, to be created in another reconciliation,
// once the first pod is deleted.
func optimizeForPVCReuse(changes Changes, state reconcile.ResourcesState) Changes {
	// pods to create will not contains pods whose creation depends on the
	// deletion of another pod, so PVC can be reused
	toCreateFiltered := make(PodsToCreate, 0, len(changes.ToCreate))

ForEachPodToCreate:
	for _, toCreate := range changes.ToCreate {
		pvcTemplates := toCreate.PodSpecCtx.NodeSpec.VolumeClaimTemplates
		if len(pvcTemplates) == 0 {
			// this pod is not using PVCs
			toCreateFiltered = append(toCreateFiltered, toCreate)
			continue
		}

	CompareWithPodsToDelete:
		for j, toDelete := range changes.ToDelete {
			if toDelete.ReusePVC {
				// PVCs already reused for another pod to create
				continue CompareWithPodsToDelete
			}
			pvcComparison := comparison.ComparePersistentVolumeClaims(toDelete.Pod.Spec.Volumes, pvcTemplates, state)
			if pvcComparison.Match {
				// PVC of that pod can be reused
				changes.ToDelete[j].ReusePVC = true
				// drop the pod to create from changes: we want to wait for
				// the deletion to happen first, so the orphaned PVC can be reused
				// the pod to create will reappear in the list of pods to create in subsequent reconciliations
				continue ForEachPodToCreate
			}
		}
		// no pvc match for that one: should be a new pod with new volumes
		toCreateFiltered = append(toCreateFiltered, toCreate)
	}
	changes.ToCreate = toCreateFiltered
	return changes
}
