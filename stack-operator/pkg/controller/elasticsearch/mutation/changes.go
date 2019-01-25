package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Changes represents the changes to perform on the Elasticsearch pods
type Changes struct {
	ToCreate []PodToCreate
	ToKeep   []corev1.Pod
	ToDelete []corev1.Pod
}

// PodToCreate defines a pod to be created, along with
// the reasons why it doesn't match any existing pod
type PodToCreate struct {
	Pod             corev1.Pod
	PodSpecCtx      pod.PodSpecContext
	MismatchReasons map[string][]string
}

// EmptyChanges creates an empty Changes with empty arrays (not nil)
func EmptyChanges() Changes {
	return Changes{
		ToCreate: []PodToCreate{},
		ToKeep:   []corev1.Pod{},
		ToDelete: []corev1.Pod{},
	}
}

// HasChanges returns true if there are no topology changes to performed
func (c Changes) HasChanges() bool {
	return len(c.ToCreate) > 0 || len(c.ToDelete) > 0
}

// HasRunningPods returns true if there are existing pods to keep. Does not say that they form a working cluster.
func (c Changes) HasRunningPods() bool {
	return len(c.ToKeep) > 0
}

// IsEmpty returns true if this set has no deletion, creation or kept pods
func (c Changes) IsEmpty() bool {
	return len(c.ToCreate) == 0 && len(c.ToDelete) == 0 && len(c.ToKeep) == 0
}

// Copy copies this Changes. It copies the underlying slices and maps, but not their contents
func (c Changes) Copy() Changes {
	res := Changes{
		ToCreate: append([]PodToCreate{}, c.ToCreate...),
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
		selector, err := metav1.LabelSelectorAsSelector(&gd.Selector)
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
	for _, toCreate := range c.ToCreate {
		if selector.Matches(labels.Set(toCreate.Pod.Labels)) {
			matchingChanges.ToCreate = append(matchingChanges.ToCreate, toCreate)
		} else {
			remainingChanges.ToCreate = append(remainingChanges.ToCreate, toCreate)
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
