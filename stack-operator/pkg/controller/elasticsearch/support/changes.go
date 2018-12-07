package support

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// Changes represents the changes to perform on the Elasticsearch pods
type Changes struct {
	ToAdd    []PodToAdd
	ToKeep   []corev1.Pod
	ToRemove []corev1.Pod
}

// sortPodByCreationTimestampAsc is a sort function for a list of pods
func sortPodByCreationTimestampAsc(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool { return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp) }
}

// PodToAdd defines a pod to be added, along with
// the reasons why it doesn't match any existing pod
type PodToAdd struct {
	PodSpecCtx      PodSpecContext
	MismatchReasons map[string][]string
}

// IsEmpty returns true if there are no topology changes to performed
func (c Changes) HasChanges() bool {
	return len(c.ToAdd) > 0 || len(c.ToRemove) > 0
}

// CalculateChanges returns Changes to perform by comparing actual pods to expected pods spec
func CalculateChanges(expectedPodSpecCtxs []PodSpecContext, state ResourcesState) (Changes, error) {
	// work on copies of the arrays, on which we can safely remove elements
	expectedCopy := make([]PodSpecContext, len(expectedPodSpecCtxs))
	copy(expectedCopy, expectedPodSpecCtxs)
	actualCopy := make([]corev1.Pod, len(state.CurrentPods))
	copy(actualCopy, state.CurrentPods)

	return mutableCalculateChanges(expectedCopy, actualCopy, state)
}

func mutableCalculateChanges(
	expectedPodSpecCtxs []PodSpecContext,
	actualPods []corev1.Pod,
	state ResourcesState,
) (Changes, error) {
	changes := Changes{
		ToAdd:    []PodToAdd{},
		ToKeep:   []corev1.Pod{},
		ToRemove: []corev1.Pod{},
	}

	for _, expectedPodSpecCtx := range expectedPodSpecCtxs {
		comparisonResult, err := getAndRemoveMatchingPod(expectedPodSpecCtx, actualPods, state)
		if err != nil {
			return changes, err
		}
		if comparisonResult.IsMatch {
			// matching pod already exists, keep it
			changes.ToKeep = append(changes.ToKeep, comparisonResult.MatchingPod)
			// one less pod to compare with
			actualPods = comparisonResult.RemainingPods
		} else {
			// no matching pod, a new one should be added
			changes.ToAdd = append(changes.ToAdd, PodToAdd{
				PodSpecCtx:      expectedPodSpecCtx,
				MismatchReasons: comparisonResult.MismatchReasonsPerPod,
			})
		}
	}
	// remaining actual pods should be removed
	changes.ToRemove = actualPods

	// sort changes for idempotent processing
	sort.SliceStable(changes.ToKeep, sortPodByCreationTimestampAsc(changes.ToKeep))
	sort.SliceStable(changes.ToRemove, sortPodByCreationTimestampAsc(changes.ToRemove))

	return changes, nil
}

type PodComparisonResult struct {
	IsMatch               bool
	MatchingPod           corev1.Pod
	MismatchReasonsPerPod map[string][]string
	RemainingPods         []corev1.Pod
}

func getAndRemoveMatchingPod(podSpecCtx PodSpecContext, pods []corev1.Pod, state ResourcesState) (PodComparisonResult, error) {
	mismatchReasonsPerPod := map[string][]string{}

	for i, pod := range pods {
		if IsTainted(pod) {
			mismatchReasonsPerPod[pod.Name] = []string{TaintedReason}
			continue
		}

		isMatch, mismatchReasons, err := podMatchesSpec(pod, podSpecCtx, state)
		if err != nil {
			return PodComparisonResult{}, err
		}
		if isMatch {
			// matching pod found
			// remove it from the remaining pods
			return PodComparisonResult{
				IsMatch:               true,
				MatchingPod:           pod,
				MismatchReasonsPerPod: mismatchReasonsPerPod,
				RemainingPods:         append(pods[:i], pods[i+1:]...),
			}, nil
		}
		mismatchReasonsPerPod[pod.Name] = mismatchReasons
	}
	// no matching pod found
	return PodComparisonResult{
		IsMatch:               false,
		MismatchReasonsPerPod: mismatchReasonsPerPod,
		RemainingPods:         pods,
	}, nil
}
