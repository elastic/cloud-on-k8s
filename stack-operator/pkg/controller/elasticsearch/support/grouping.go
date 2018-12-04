package support

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type ChangeSet struct {
	ToRemove []corev1.Pod
	ToAdd    []corev1.Pod
	ToKeep   []corev1.Pod

	ToAddContext map[string]PodToAdd
}

// IsEmpty returns true if there are no topology changes to performed
func (c ChangeSet) IsEmpty() bool {
	return len(c.ToAdd) == 0 && len(c.ToRemove) == 0
}

type GroupedChangeSet struct {
	Definition v1alpha1.GroupingDefinition
	PodsState  PodsState
	ChangeSet  ChangeSet
}

func CreateGroupedChangeSets(
	remainingChanges ChangeSet,
	remainingPodsState PodsState,
	groupingDefinitions []v1alpha1.GroupingDefinition,
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

		toRemove, toRemoveRemaining := selectPods(selector, remainingChanges.ToRemove)
		remainingChanges.ToRemove = toRemoveRemaining

		toAdd, toAddRemaining := selectPods(selector, remainingChanges.ToAdd)
		remainingChanges.ToAdd = toAddRemaining

		toKeep, toKeepRemaining := selectPods(selector, remainingChanges.ToKeep)
		remainingChanges.ToKeep = toKeepRemaining

		groupedChanges.ChangeSet.ToKeep = toKeep
		groupedChanges.ChangeSet.ToRemove = toRemove
		groupedChanges.ChangeSet.ToAdd = toAdd

		toAddContext := make(map[string]PodToAdd, len(groupedChanges.ChangeSet.ToAdd))
		for _, pod := range groupedChanges.ChangeSet.ToAdd {
			toAddContext[pod.Name] = remainingChanges.ToAddContext[pod.Name]
			delete(remainingChanges.ToAddContext, pod.Name)
		}
		groupedChanges.ChangeSet.ToAddContext = toAddContext

		var podsState PodsState
		podsState, remainingPodsState = remainingPodsState.partition(groupedChanges.ChangeSet)
		groupedChanges.PodsState = podsState

		groupedChangeSets[i] = groupedChanges
	}

	if !remainingChanges.IsEmpty() || len(remainingChanges.ToKeep) > 0 {
		// add a catch-all with the remainder if non-empty
		groupedChangeSets = append(groupedChangeSets, GroupedChangeSet{
			Definition: v1alpha1.DefaultFallbackGroupingDefinition,
			PodsState:  remainingPodsState,
			ChangeSet:  remainingChanges,
		})
	}

	return groupedChangeSets, nil
}

func selectPods(selector labels.Selector, remainingPods []corev1.Pod) ([]corev1.Pod, []corev1.Pod) {
	selectedPods := make([]corev1.Pod, 0)
	for i := len(remainingPods) - 1; i >= 0; i-- {
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

type PodsState struct {
	Pending     map[string]corev1.Pod
	Joining     map[string]corev1.Pod
	Operational map[string]corev1.Pod
	Migrating   map[string]corev1.Pod
	Deleting    map[string]corev1.Pod
}

func NewEmptyPodsState() PodsState {
	return PodsState{
		Pending:     make(map[string]corev1.Pod),
		Joining:     make(map[string]corev1.Pod),
		Operational: make(map[string]corev1.Pod),
		Migrating:   make(map[string]corev1.Pod),
		Deleting:    make(map[string]corev1.Pod),
	}
}

func (s PodsState) partition(changeSet ChangeSet) (PodsState, PodsState) {
	current := NewEmptyPodsState()
	remaining := s
	// no need to consider changeSet.ToAdd here, as they will not exist in a PodsState
	for _, pods := range [][]corev1.Pod{changeSet.ToRemove, changeSet.ToKeep} {
		var partialState PodsState
		partialState, remaining = remaining.partitionByPods(pods)
		current = current.mergeWith(partialState)
	}
	return current, remaining
}

func (s PodsState) partitionByPods(pods []corev1.Pod) (PodsState, PodsState) {
	current := NewEmptyPodsState()
	for _, pod := range pods {
		if _, ok := s.Pending[pod.Name]; ok {
			current.Pending[pod.Name] = pod
			delete(s.Pending, pod.Name)
			continue
		}
		if _, ok := s.Joining[pod.Name]; ok {
			current.Joining[pod.Name] = pod
			delete(s.Joining, pod.Name)
			continue
		}
		if _, ok := s.Operational[pod.Name]; ok {
			current.Operational[pod.Name] = pod
			delete(s.Operational, pod.Name)
			continue
		}
		if _, ok := s.Migrating[pod.Name]; ok {
			current.Migrating[pod.Name] = pod
			delete(s.Migrating, pod.Name)
			continue
		}
		if _, ok := s.Deleting[pod.Name]; ok {
			current.Deleting[pod.Name] = pod
			delete(s.Deleting, pod.Name)
			continue
		}
		log.Info("Unable to find pod in pods state", "pod_name", pod.Name)
	}

	return current, s
}

func (s PodsState) mergeWith(other PodsState) PodsState {

	for k, v := range other.Pending {
		s.Pending[k] = v
	}

	for k, v := range other.Joining {
		s.Joining[k] = v
	}

	for k, v := range other.Operational {
		s.Operational[k] = v
	}

	for k, v := range other.Migrating {
		s.Migrating[k] = v
	}

	for k, v := range other.Deleting {
		s.Deleting[k] = v
	}

	return s
}

func NewPodsState(
	resourcesState ResourcesState,
	observedState ObservedState,
	changes Changes,
) PodsState {
	podsState := NewEmptyPodsState()
	// pendingPods are pods that have been created in the API but is not scheduled or running yet.
	pendingPods, _ := resourcesState.CurrentPodsByPhase[corev1.PodPending]
	for _, pod := range pendingPods {
		podsState.Pending[pod.Name] = pod
	}

	// joiningPods are running pods that are not seen in the observed cluster state
	// XXX: requires an observerState.ClusterState to work, which is not reflected in the variables set
	if observedState.ClusterState != nil {
		nodesByName := make(map[string]client.Node, len(observedState.ClusterState.Nodes))
		for _, node := range observedState.ClusterState.Nodes {
			nodesByName[node.Name] = node
		}

		for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodRunning] {
			// if the pod is not known in the cluster state, we assume it's supposed to join
			if _, ok := nodesByName[currentRunningPod.Name]; !ok {
				podsState.Joining[currentRunningPod.Name] = currentRunningPod
			} else {
				podsState.Operational[currentRunningPod.Name] = currentRunningPod
			}
		}
	}

	// migratingPods are pods that are being actively migrated away from.
	// this is equal to changes.ToRemove for now, but could change to a sub-selection based on a future strategy
	migratingPods := changes.ToRemove
	for _, pod := range migratingPods {
		podsState.Migrating[pod.Name] = pod
	}

	// leavingPods = migratingPods that is primed for deletion, but not deleted yet (is this even a thing?)

	// deletingPods are pods we have issued a delete request for, but haven't disappeared from the API yet
	deletingPods := resourcesState.DeletingPods
	for _, pod := range deletingPods {
		podsState.Deleting[pod.Name] = pod
	}

	return podsState
}

func PodListToNames(pods []corev1.Pod) []string {
	names := make([]string, len(pods))
	for i, pod := range pods {
		names[i] = pod.Name
	}
	return names
}

func PodMapToNames(pods map[string]corev1.Pod) []string {
	names := make([]string, len(pods))
	i := 0
	for k := range pods {
		names[i] = k
		i++
	}
	return names
}
