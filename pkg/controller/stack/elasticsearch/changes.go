package elasticsearch

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

// SortPodByName is a sort function for a list of pods
func SortPodByName(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool { return pods[i].Name < pods[j].Name }
}

// PodToAdd defines a pod to be added, along with
// the reasons why it doesn't match any existing pod
type PodToAdd struct {
	PodSpecCtx      PodSpecContext
	MismatchReasons map[string][]string
}

// ShouldMigrate returns true if there are some topology changes to performed
func (c Changes) ShouldMigrate() bool {
	return len(c.ToAdd) != 0 || len(c.ToRemove) != 0
}

// CalculateChanges returns Changes to perform by comparing actual pods to expected pods spec
func CalculateChanges(expectedPodSpecCtxs []PodSpecContext, actual []corev1.Pod) (Changes, error) {
	// work on copies of the arrays, on which we can safely remove elements
	expectedCopy := make([]PodSpecContext, len(expectedPodSpecCtxs))
	copy(expectedCopy, expectedPodSpecCtxs)
	actualCopy := make([]corev1.Pod, len(actual))
	copy(actualCopy, actual)

	return mutableCalculateChanges(expectedCopy, actualCopy)
}

func mutableCalculateChanges(expectedPodSpecCtxs []PodSpecContext, actualPods []corev1.Pod) (Changes, error) {
	changes := Changes{
		ToAdd:    []PodToAdd{},
		ToKeep:   []corev1.Pod{},
		ToRemove: []corev1.Pod{},
	}

	for _, expectedPodSpecCtx := range expectedPodSpecCtxs {
		comparisonResult, err := getAndRemoveMatchingPod(expectedPodSpecCtx, actualPods)
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
	// TODO: smart sort  to process nodes in a particular order
	sort.SliceStable(changes.ToKeep, SortPodByName(changes.ToKeep))
	sort.SliceStable(changes.ToRemove, SortPodByName(changes.ToRemove))

	return changes, nil
}

type PodComparisonResult struct {
	IsMatch               bool
	MatchingPod           corev1.Pod
	MismatchReasonsPerPod map[string][]string
	RemainingPods         []corev1.Pod
}

func getAndRemoveMatchingPod(podSpecCtx PodSpecContext, pods []corev1.Pod) (PodComparisonResult, error) {
	mismatchReasonsPerPod := map[string][]string{}

	for i, pod := range pods {
		isMatch, mismatchReasons, err := podMatchesSpec(pod, podSpecCtx)
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
