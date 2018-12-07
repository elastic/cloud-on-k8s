package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ChangeSet contains pods that should be removed, added or kept in during reconciliation.
type ChangeSet struct {
	// ToRemove is a list of pods that should eventually be removed
	ToRemove []corev1.Pod
	// ToAdd is a a list of pods that should be created. Each pod in this list will have a corresponding entry in
	// ToAddContext by pod.Name
	ToAdd []corev1.Pod
	// ToKeep is a list of pods that should not be changed.
	ToKeep []corev1.Pod

	// ToAddContext contains context for added pods, which is required when creating the pods and associated resources
	// using the Kubernetes API.
	ToAddContext map[string]support.PodToAdd
}

// IsEmpty returns true if this set has no removal, additions or kept pods.
func (s ChangeSet) IsEmpty() bool {
	return len(s.ToAdd) == 0 && len(s.ToRemove) == 0 && len(s.ToKeep) == 0
}

// Copy copies this ChangeSet. It copies the underlying slices and maps, but not their contents.
func (s ChangeSet) Copy() ChangeSet {
	res := ChangeSet{
		ToAdd:        append([]corev1.Pod(nil), s.ToAdd...),
		ToAddContext: make(map[string]support.PodToAdd, len(s.ToAddContext)),
		ToKeep:       append([]corev1.Pod(nil), s.ToKeep...),
		ToRemove:     append([]corev1.Pod(nil), s.ToRemove...),
	}

	for k, v := range s.ToAddContext {
		res.ToAddContext[k] = v
	}

	return res
}

// NewPodFunc is a function that is able to create pods from a PodSpecContext
type NewPodFunc func(ctx support.PodSpecContext) (corev1.Pod, error)

// NewChangeSetFromChanges derives a single ChangeSet that carries over all the changes as a single ChangeSet.
func NewChangeSetFromChanges(changes support.Changes, newPodFunc NewPodFunc) (*ChangeSet, error) {
	podsToAdd := make([]corev1.Pod, len(changes.ToAdd))
	toAddContext := make(map[string]support.PodToAdd, len(changes.ToAdd))
	for i, podToAdd := range changes.ToAdd {
		pod, err := newPodFunc(podToAdd.PodSpecCtx)
		if err != nil {
			return nil, err
		}
		podsToAdd[i] = pod
		toAddContext[pod.Name] = podToAdd
	}

	cs := ChangeSet{
		ToRemove: changes.ToRemove[:],
		ToAdd:    podsToAdd,
		ToKeep:   changes.ToKeep[:],

		ToAddContext: toAddContext,
	}

	return &cs, nil
}

// Group groups the provided ChangeSet into groups based on the GroupingDefinitions
func (s ChangeSet) Group(
	groupingDefinitions []v1alpha1.GroupingDefinition,
	remainingPodsState PodsState,
) (GroupedChangeSets, error) {
	remainingChangeSet := s.Copy()
	groupedChangeSets := make([]GroupedChangeSet, 0, len(groupingDefinitions)+1)

	for i, gd := range groupingDefinitions {
		groupedChanges := GroupedChangeSet{
			Name: indexedGroupName(i),
		}
		selector, err := v1.LabelSelectorAsSelector(&gd.Selector)
		if err != nil {
			return nil, err
		}

		groupedChanges.ChangeSet.ToRemove, remainingChangeSet.ToRemove =
			partitionPodsBySelector(selector, remainingChangeSet.ToRemove)
		groupedChanges.ChangeSet.ToAdd, remainingChangeSet.ToAdd =
			partitionPodsBySelector(selector, remainingChangeSet.ToAdd)
		groupedChanges.ChangeSet.ToKeep, remainingChangeSet.ToKeep =
			partitionPodsBySelector(selector, remainingChangeSet.ToKeep)

		if groupedChanges.ChangeSet.IsEmpty() {
			continue
		}

		toAddContext := make(map[string]support.PodToAdd, len(groupedChanges.ChangeSet.ToAdd))
		for _, pod := range groupedChanges.ChangeSet.ToAdd {
			toAddContext[pod.Name] = remainingChangeSet.ToAddContext[pod.Name]
			delete(remainingChangeSet.ToAddContext, pod.Name)
		}
		groupedChanges.ChangeSet.ToAddContext = toAddContext

		var podsState PodsState
		podsState, remainingPodsState = remainingPodsState.Partition(groupedChanges.ChangeSet)
		groupedChanges.PodsState = podsState

		groupedChangeSets = append(groupedChangeSets, groupedChanges)
	}

	if !remainingChangeSet.IsEmpty() {
		// remaining changes do not match any group definition selector, group them together as a single group
		groupedChangeSets = append(groupedChangeSets, GroupedChangeSet{
			Name:      UnmatchedGroupName,
			PodsState: remainingPodsState,
			ChangeSet: remainingChangeSet,
		})
	}

	return groupedChangeSets, nil
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
