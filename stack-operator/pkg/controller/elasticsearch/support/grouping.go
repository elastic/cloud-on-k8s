package support

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)
import corev1 "k8s.io/api/core/v1"

type GroupingDefinition struct {
	Selector v1.LabelSelector
	// strategy goes here..
}

type ChangeSet struct {
	ToRemove []corev1.Pod
	ToAdd    []corev1.Pod
	ToKeep   []corev1.Pod
}

// IsEmpty returns true if there are no topology changes to performed
func (c ChangeSet) IsEmpty() bool {
	return len(c.ToAdd) == 0 && len(c.ToRemove) == 0
}

type GroupedChangeSet struct {
	Definition GroupingDefinition
	ChangeSet  ChangeSet
}

func CreateGroupedChangeSets(
	remainingChanges ChangeSet,
	groupingDefinitions []GroupingDefinition,
) ([]GroupedChangeSet, error) {
	groupedChangeSets := make([]GroupedChangeSet, len(groupingDefinitions))

	for i, gd := range groupingDefinitions {
		groupedChanges := GroupedChangeSet{
			Definition: gd,
		}
		selector, err := v1.LabelSelectorAsSelector(&gd.Selector)
		if err != nil {
			return nil, err
		}

		selectPods := func(selector labels.Selector, remainingPods []corev1.Pod) ([]corev1.Pod, []corev1.Pod) {
			selectedPods := make([]corev1.Pod, 0)
			for i := len(remainingPods); i >= 0; i-- {
				pod := remainingPods[i]

				podLabels := labels.Set(pod.Labels)
				if selector.Matches(podLabels) {
					selectedPods = append(selectedPods, pod)

					remainingPods = append(remainingPods[:i], remainingPods[i+1:]...)
				}
			}

			// reverse the selected pods slice because our reverse order-iteration above reversed it
			for i := len(selectedPods)/2 - 1; i >= 0; i-- {
				opp := len(selectedPods) - 1 - i
				selectedPods[i], selectedPods[opp] = selectedPods[opp], selectedPods[i]
			}

			return selectedPods, remainingPods
		}

		toRemove, toRemoveRemaining := selectPods(selector, remainingChanges.ToRemove)
		remainingChanges.ToRemove = toRemoveRemaining

		toAdd, toAddRemaining := selectPods(selector, remainingChanges.ToAdd)
		remainingChanges.ToAdd = toAddRemaining

		toKeep, toKeepRemaining := selectPods(selector, remainingChanges.ToKeep)
		remainingChanges.ToKeep = toKeepRemaining

		groupedChanges.ChangeSet.ToKeep = toKeep
		groupedChanges.ChangeSet.ToRemove = toRemove
		groupedChanges.ChangeSet.ToAdd = toAdd

		groupedChangeSets[i] = groupedChanges
	}
	return groupedChangeSets, nil
}

type PodsState struct {
	Pending     map[string]corev1.Pod
	Joining     map[string]corev1.Pod
	Operational map[string]corev1.Pod
	Migrating   map[string]corev1.Pod
	Deleting    map[string]corev1.Pod
}
