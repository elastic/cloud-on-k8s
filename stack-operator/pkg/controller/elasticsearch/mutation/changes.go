package mutation

import (
	"sort"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// PodBuilder is a function that is able to create pods from a PodSpecContext,
// mostly used by the various supported versions
type PodBuilder func(ctx support.PodSpecContext) (corev1.Pod, error)

// Changes represents the changes to perform on the Elasticsearch pods
type Changes struct {
	ToAdd    []PodToAdd
	ToKeep   []corev1.Pod
	ToRemove []corev1.Pod
}

// EmptyChanges creates an empty Changes with empty arrays not nil
func EmptyChanges() Changes {
	return Changes{
		ToAdd:    []PodToAdd{},
		ToKeep:   []corev1.Pod{},
		ToRemove: []corev1.Pod{},
	}
}

// sortPodByCreationTimestampAsc is a sort function for a list of pods
func sortPodByCreationTimestampAsc(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool { return pods[i].CreationTimestamp.Before(&pods[j].CreationTimestamp) }
}

// PodToAdd defines a pod to be added, along with
// the reasons why it doesn't match any existing pod
type PodToAdd struct {
	Pod             corev1.Pod
	PodSpecCtx      support.PodSpecContext
	MismatchReasons map[string][]string
}

// HasChanges returns true if there are no topology changes to performed
func (c Changes) HasChanges() bool {
	return len(c.ToAdd) > 0 || len(c.ToRemove) > 0
}

// IsEmpty returns true if this set has no removal, additions or kept pods.
func (c Changes) IsEmpty() bool {
	return len(c.ToAdd) == 0 && len(c.ToRemove) == 0 && len(c.ToKeep) == 0
}

// Copy copies this Changes. It copies the underlying slices and maps, but not their contents.
func (c Changes) Copy() Changes {
	res := Changes{
		ToAdd:    append([]PodToAdd{}, c.ToAdd...),
		ToKeep:   append([]corev1.Pod{}, c.ToKeep...),
		ToRemove: append([]corev1.Pod{}, c.ToRemove...),
	}
	return res
}

// Group groups the current changes into groups based on the GroupingDefinitions
func (c Changes) Group(
	groupingDefinitions []v1alpha1.GroupingDefinition,
	remainingPodsState PodsState,
) (ChangeGroups, error) {
	remainingChanges := c.Copy()
	groups := make([]ChangeGroup, 0, len(groupingDefinitions)+1)

	for i, gd := range groupingDefinitions {
		group := ChangeGroup{
			Name: indexedGroupName(i),
		}
		selector, err := v1.LabelSelectorAsSelector(&gd.Selector)
		if err != nil {
			return nil, err
		}

		group.Changes, remainingChanges = remainingChanges.Partition(selector)
		if group.Changes.IsEmpty() {
			// selector does not match anything
			continue
		}

		group.PodsState, remainingPodsState = remainingPodsState.Partition(group.Changes)

		groups = append(groups, group)
	}

	if !remainingChanges.IsEmpty() {
		// remaining changes do not match any group definition selector, group them together as a single group
		groups = append(groups, ChangeGroup{
			Name:      UnmatchedGroupName,
			PodsState: remainingPodsState,
			Changes:   remainingChanges,
		})
	}

	return groups, nil
}

// Partition divides changes into 2 changes based on the given selector:
// changes that match the selector, and changes that don't
func (c Changes) Partition(selector labels.Selector) (Changes, Changes) {
	matchingChanges := EmptyChanges()
	remainingChanges := EmptyChanges()

	matchingChanges.ToKeep, remainingChanges.ToKeep = partitionPodsBySelector(selector, c.ToKeep)
	matchingChanges.ToRemove, remainingChanges.ToRemove = partitionPodsBySelector(selector, c.ToRemove)

	for _, toAdd := range c.ToAdd {
		if selector.Matches(labels.Set(toAdd.Pod.Labels)) {
			matchingChanges.ToAdd = append(matchingChanges.ToAdd, toAdd)
		} else {
			remainingChanges.ToAdd = append(remainingChanges.ToAdd, toAdd)
		}
	}

	return matchingChanges, remainingChanges
}

// partitionPodsBySelector partitions pods into two sets: one for pods matching the selector and one for the rest. it
// guarantees that the order of the pods are not changed.
func partitionPodsBySelector(selector labels.Selector, pods []corev1.Pod) ([]corev1.Pod, []corev1.Pod) {
	matchingPods := make([]corev1.Pod, 0, len(pods))
	remainingPods := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			matchingPods = append(matchingPods, pod)
		} else {
			remainingPods = append(remainingPods, pod)
		}
	}
	return matchingPods, remainingPods
}

// CalculateChanges returns Changes to perform by comparing actual pods to expected pods spec
func CalculateChanges(expectedPodSpecCtxs []support.PodSpecContext, state support.ResourcesState, podBuilder PodBuilder) (Changes, error) {
	// work on copies of the arrays, on which we can safely remove elements
	expectedCopy := make([]support.PodSpecContext, len(expectedPodSpecCtxs))
	copy(expectedCopy, expectedPodSpecCtxs)
	actualCopy := make([]corev1.Pod, len(state.CurrentPods))
	copy(actualCopy, state.CurrentPods)

	return mutableCalculateChanges(expectedCopy, actualCopy, state, podBuilder)
}

func mutableCalculateChanges(
	expectedPodSpecCtxs []support.PodSpecContext,
	actualPods []corev1.Pod,
	state support.ResourcesState,
	podBuilder PodBuilder,
) (Changes, error) {
	changes := EmptyChanges()

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
			pod, err := podBuilder(expectedPodSpecCtx)
			if err != nil {
				return changes, err
			}
			changes.ToAdd = append(changes.ToAdd, PodToAdd{
				Pod:             pod,
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

func getAndRemoveMatchingPod(podSpecCtx support.PodSpecContext, pods []corev1.Pod, state support.ResourcesState) (PodComparisonResult, error) {
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
