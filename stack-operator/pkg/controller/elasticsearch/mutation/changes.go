package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Changes represents the changes to perform on the Elasticsearch pods
type Changes struct {
	ToAdd    []PodToAdd
	ToKeep   []corev1.Pod
	ToDelete []corev1.Pod
}

// PodToAdd defines a pod to be added, along with
// the reasons why it doesn't match any existing pod
type PodToAdd struct {
	Pod             corev1.Pod
	PodSpecCtx      support.PodSpecContext
	MismatchReasons map[string][]string
}

// EmptyChanges creates an empty Changes with empty arrays (not nil)
func EmptyChanges() Changes {
	return Changes{
		ToAdd:    []PodToAdd{},
		ToKeep:   []corev1.Pod{},
		ToDelete: []corev1.Pod{},
	}
}

// HasChanges returns true if there are no topology changes to performed
func (c Changes) HasChanges() bool {
	return len(c.ToAdd) > 0 || len(c.ToDelete) > 0
}

// IsEmpty returns true if this set has no removal, additions or kept pods.
func (c Changes) IsEmpty() bool {
	return len(c.ToAdd) == 0 && len(c.ToDelete) == 0 && len(c.ToKeep) == 0
}

// Copy copies this Changes. It copies the underlying slices and maps, but not their contents.
func (c Changes) Copy() Changes {
	res := Changes{
		ToAdd:    append([]PodToAdd{}, c.ToAdd...),
		ToKeep:   append([]corev1.Pod{}, c.ToKeep...),
		ToDelete: append([]corev1.Pod{}, c.ToDelete...),
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
	matchingChanges.ToDelete, remainingChanges.ToDelete = partitionPodsBySelector(selector, c.ToDelete)
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
