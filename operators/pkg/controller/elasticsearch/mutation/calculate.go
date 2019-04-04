// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"sort"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation/comparison"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	corev1 "k8s.io/api/core/v1"
)

// PodBuilder is a function that is able to create pods from a PodSpecContext,
// mostly used by the various supported versions
type PodBuilder func(ctx pod.PodSpecContext) (corev1.Pod, error)

// PodComparisonResult holds information about pod comparison result
type PodComparisonResult struct {
	IsMatch               bool
	MatchingPod           pod.PodWithConfig
	MismatchReasonsPerPod map[string][]string
	RemainingPods         pod.PodsWithConfig
}

// CalculateChanges returns Changes to perform by comparing actual pods to expected pods spec
func CalculateChanges(
	expectedPodSpecCtxs []pod.PodSpecContext,
	state reconcile.ResourcesState,
	podBuilder PodBuilder,
) (Changes, error) {
	// work on copies of the arrays, on which we can safely remove elements
	expectedCopy := make([]pod.PodSpecContext, len(expectedPodSpecCtxs))
	copy(expectedCopy, expectedPodSpecCtxs)
	actualCopy := make(pod.PodsWithConfig, len(state.CurrentPods))
	copy(actualCopy, state.CurrentPods)
	deletingCopy := make(pod.PodsWithConfig, len(state.DeletingPods))
	copy(deletingCopy, state.DeletingPods)

	return mutableCalculateChanges(expectedCopy, actualCopy, state, podBuilder, deletingCopy)
}

func mutableCalculateChanges(
	expectedPodSpecCtxs []pod.PodSpecContext,
	actualPods pod.PodsWithConfig,
	state reconcile.ResourcesState,
	podBuilder PodBuilder,
	deletingPods pod.PodsWithConfig,
) (Changes, error) {
	changes := EmptyChanges() // resulting changes

	for _, expectedPodSpecCtx := range expectedPodSpecCtxs {

		// look for a matching pod in the current ones
		actualComparisonResult, err := getAndRemoveMatchingPod(expectedPodSpecCtx, actualPods, state)
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
		deletingComparisonResult, err := getAndRemoveMatchingPod(expectedPodSpecCtx, deletingPods, state)
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
		pod, err := podBuilder(expectedPodSpecCtx)
		if err != nil {
			return changes, err
		}
		changes.ToCreate = append(changes.ToCreate, PodToCreate{
			Pod:             pod,
			PodSpecCtx:      expectedPodSpecCtx,
			MismatchReasons: actualComparisonResult.MismatchReasonsPerPod,
		})
	}
	// remaining actual pods should be deleted
	changes.ToDelete = actualPods

	// sort changes for idempotent processing
	sort.SliceStable(changes.ToKeep, sortPodByCreationTimestampAsc(changes.ToKeep))
	sort.SliceStable(changes.ToDelete, sortPodByCreationTimestampAsc(changes.ToDelete))

	targetLicense, requiresFullRestart := licenseChangeRequiresFullClusterRestart(actualPods, expectedPodSpecCtxs)
	if requiresFullRestart {
		changes = adaptChangesForFullClusterRestart(changes, targetLicense)
	}

	return changes, nil
}

func getAndRemoveMatchingPod(podSpecCtx pod.PodSpecContext, podsWithConfig pod.PodsWithConfig, state reconcile.ResourcesState) (PodComparisonResult, error) {
	mismatchReasonsPerPod := map[string][]string{}

	for i, podWithConfig := range podsWithConfig {
		pod := podWithConfig.Pod

		// check if the pod matches the expected spec
		isMatch, mismatchReasons := comparison.PodMatchesSpec(podWithConfig, podSpecCtx, state)
		if !isMatch {
			mismatchReasonsPerPod[pod.Name] = mismatchReasons
			continue
		}

		// check if the pod config matches the expected config
		cfgComparison := comparison.CompareConfigs(podWithConfig.Config, podSpecCtx.Config)
		if !cfgComparison.Match {
			mismatchReasonsPerPod[pod.Name] = cfgComparison.MismatchReasons
			continue
		}

		// match found
		return PodComparisonResult{
			IsMatch:               true,
			MatchingPod:           podWithConfig,
			MismatchReasonsPerPod: mismatchReasonsPerPod,
			// remove the matching pod from the remaining pods
			RemainingPods: append(podsWithConfig[:i], podsWithConfig[i+1:]...),
		}, nil
	}

	// no matching pod found
	return PodComparisonResult{
		IsMatch:               false,
		MismatchReasonsPerPod: mismatchReasonsPerPod,
		RemainingPods:         podsWithConfig,
	}, nil
}

// licenseChangeRequiresFullClusterRestart returns true if mutation from actual to expected pods
// requires to restart actual pods with a different config first.
func licenseChangeRequiresFullClusterRestart(actualPods pod.PodsWithConfig, expectedPodSpecCtxs []pod.PodSpecContext) (v1alpha1.LicenseType, bool) {
	if len(actualPods) == 0 || len(expectedPodSpecCtxs) == 0 {
		return v1alpha1.LicenseType(""), false
	}

	// Switching from TLS to non-TLS requires a full cluster restart.
	// That's actually switching from a self-gen basic license to a non-basic license,
	// or the other way around.

	// all expected pods have the same self-gen license, grab it from the first one
	targetSelfGenLicense := expectedPodSpecCtxs[0].Config[settings.XPackLicenseSelfGeneratedType] // can be empty

	// actual pods normally all have the same license, unless a migration is already in progress
	// return true if there is at least one mismatch
	currentSelfGenLicenses := make([]string, len(actualPods))
	for i, p := range actualPods {
		currentSelfGenLicenses[i] = p.Config[settings.XPackLicenseSelfGeneratedType] // can be empty
	}

	// detect a migration towards or away from basic
	basic := v1alpha1.LicenseTypeBasic.String()
	for _, currentSelfGenLicense := range currentSelfGenLicenses {
		if (currentSelfGenLicense == basic && targetSelfGenLicense != basic) ||
			(currentSelfGenLicense != basic && targetSelfGenLicense == basic) {
			return v1alpha1.LicenseType(targetSelfGenLicense), true
		}
	}

	return "", false
}
