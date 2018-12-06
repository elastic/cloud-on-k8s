package support

import (
	"fmt"
	"sort"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ChangeSet contains pods that should be remove
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
	ToAddContext map[string]PodToAdd
}

// IsEmpty returns true if this set has no removal, additions or kept pods.
func (s ChangeSet) IsEmpty() bool {
	return len(s.ToAdd) == 0 && len(s.ToRemove) == 0 && len(s.ToKeep) == 0
}

// NewPodFunc is a function that is able to create pods from a PodSpecContext
type NewPodFunc func(ctx PodSpecContext) (corev1.Pod, error)

// NewChangeSetFromChanges derives a single ChangeSet that carries over all the changes as a single ChangeSet.
func NewChangeSetFromChanges(changes Changes, newPodFunc NewPodFunc) (*ChangeSet, error) {
	podsToAdd := make([]corev1.Pod, len(changes.ToAdd))
	toAddContext := make(map[string]PodToAdd, len(changes.ToAdd))
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

// GroupedChangeSet is a ChangeSet for a specific group of pods.
type GroupedChangeSet struct {
	// Definition is the grouping definition that was used when creating the group
	Definition v1alpha1.GroupingDefinition
	// ChangeSet contains the changes in this group
	ChangeSet ChangeSet
	// PodsState contains the state of all the pods in this group.
	PodsState PodsState
}

// KeyNumbers contains key numbers for a GroupedChangeSet, used to execute an upgrade strategy
type KeyNumbers struct {
	// TargetPods is the number of pods we should have in the final state.
	TargetPods int `json:"targetPods"`
	// CurrentPods is the current number of pods in the cluster that be using resources.
	CurrentPods int `json:"currentPods"`
	// CurrentSurge is the number of pods above the target the cluster is using.
	CurrentSurge int `json:"currentSurge"`
	// CurrentOperationalPods is the number of pods that are running and have joined the current master.
	CurrentOperationalPods int `json:"currentOperationalPods"`
	// CurrentUnavailable is the number of pods below the target the cluster is currently using.
	CurrentUnavailable int `json:"currentUnavailable"`
}

// KeyNumbers calculates and returns the KeyNumbers for this grouped change set,
func (s GroupedChangeSet) KeyNumbers() KeyNumbers {
	// when we're done, we should have ToKeep + ToAdd pods left
	targetPodsCount := len(s.ChangeSet.ToKeep) + len(s.ChangeSet.ToAdd)

	currentPodsCount := s.PodsState.CurrentPodsCount()

	// surge is the number of pods potentially consuming any resources we currently have above the target
	currentSurge := currentPodsCount - targetPodsCount

	currentOperationalPodsCount := len(s.PodsState.RunningReady)

	// unavailable is the number of fully operational pods we have below the target
	currentUnavailable := targetPodsCount - currentOperationalPodsCount

	return KeyNumbers{
		TargetPods:             targetPodsCount,
		CurrentPods:            currentPodsCount,
		CurrentSurge:           currentSurge,
		CurrentOperationalPods: currentOperationalPodsCount,
		CurrentUnavailable:     currentUnavailable,
	}
}

// GroupedChangeSets is a list GroupedChangeSet that can be validated for consistency
type GroupedChangeSets []GroupedChangeSet

// ValidateMasterChanges validates that only one changeset contains master nodes and returns an error otherwise.
func (s GroupedChangeSets) ValidateMasterChanges() error {
	// validate that only one changeset contains master nodes as a safeguard
	// TODO: this may be significantly relaxed when there's parallelizable groups etc, as well as allowing to remove
	// pods that are in a terminal phase.
	var changeSetsWithMasterNodes []int
	for i, cs := range s {
		for _, pod := range cs.ChangeSet.ToRemove {
			if NodeTypesMasterLabelName.HasValue(true, pod.Labels) {
				changeSetsWithMasterNodes = append(changeSetsWithMasterNodes, i)
				break
			}
		}
	}
	if len(changeSetsWithMasterNodes) > 1 {
		return fmt.Errorf(
			"only one group is allowed to contain master nodes, found master nodes in: %v",
			changeSetsWithMasterNodes,
		)
	}
	return nil
}

// Group groups the provided ChangeSet into groups based on the GroupingDefinitions
func (s ChangeSet) Group(
	groupingDefinitions []v1alpha1.GroupingDefinition,
	remainingPodsState PodsState,
) (GroupedChangeSets, error) {
	remainingChangeSet := s
	groupedChangeSets := make([]GroupedChangeSet, len(groupingDefinitions))

	// ensure we remove the master node last in this changeset
	sort.SliceStable(
		remainingChangeSet.ToRemove,
		sortPodsByMasterNodeLastAndCreationTimestampAsc(
			remainingPodsState.MasterNodePod,
			remainingChangeSet.ToRemove,
		),
	)

	// ensure we add master nodes first in this changeset
	sort.SliceStable(
		remainingChangeSet.ToAdd,
		sortPodsByMasterNodesFirstThenNameAsc(remainingChangeSet.ToAdd),
	)

	for i, gd := range groupingDefinitions {
		groupedChanges := GroupedChangeSet{
			Definition: gd,
		}
		selector, err := v1.LabelSelectorAsSelector(&gd.Selector)
		if err != nil {
			return nil, err
		}

		toRemove, toRemoveRemaining := partitionPodsBySelector(selector, remainingChangeSet.ToRemove)
		remainingChangeSet.ToRemove = toRemoveRemaining

		toAdd, toAddRemaining := partitionPodsBySelector(selector, remainingChangeSet.ToAdd)
		remainingChangeSet.ToAdd = toAddRemaining

		toKeep, toKeepRemaining := partitionPodsBySelector(selector, remainingChangeSet.ToKeep)
		remainingChangeSet.ToKeep = toKeepRemaining

		groupedChanges.ChangeSet.ToKeep = toKeep
		groupedChanges.ChangeSet.ToRemove = toRemove
		groupedChanges.ChangeSet.ToAdd = toAdd

		toAddContext := make(map[string]PodToAdd, len(groupedChanges.ChangeSet.ToAdd))
		for _, pod := range groupedChanges.ChangeSet.ToAdd {
			toAddContext[pod.Name] = remainingChangeSet.ToAddContext[pod.Name]
			delete(remainingChangeSet.ToAddContext, pod.Name)
		}
		groupedChanges.ChangeSet.ToAddContext = toAddContext

		var podsState PodsState
		podsState, remainingPodsState = remainingPodsState.Partition(groupedChanges.ChangeSet)
		groupedChanges.PodsState = podsState

		groupedChangeSets[i] = groupedChanges
	}

	if !remainingChangeSet.IsEmpty() {
		// add a catch-all with the remainder if non-empty
		groupedChangeSets = append(groupedChangeSets, GroupedChangeSet{
			Definition: v1alpha1.DefaultFallbackGroupingDefinition,
			PodsState:  remainingPodsState,
			ChangeSet:  remainingChangeSet,
		})
	}

	return groupedChangeSets, nil
}

// sortPodsByMasterNodeLastAndCreationTimestampAsc sorts pods in a preferred deletion order: current master node always
// last, remaining pods by oldest first.
func sortPodsByMasterNodeLastAndCreationTimestampAsc(masterNode *corev1.Pod, pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool {
		iPod := pods[i]
		jPod := pods[j]

		// sort the master node last
		if masterNode != nil {
			if iPod.Name == masterNode.Name {
				// i is the master node, so it should be last
				return false
			}
			if jPod.Name == masterNode.Name {
				// j is the master node, so it should be last
				return true
			}
		}
		// if neither is the master node, fall back to sorting by creation timestamp, removing the oldest first.
		return iPod.CreationTimestamp.Before(&jPod.CreationTimestamp)
	}
}

// sortPodsByMasterNodesFirstThenNameAsc sorts pods in a preferred creation order: master nodes first, then by name
// the by name part is used to ensure a stable sort order.
func sortPodsByMasterNodesFirstThenNameAsc(pods []corev1.Pod) func(i, j int) bool {
	return func(i, j int) bool {
		// sort by master nodes last
		iPod := pods[i]
		jPod := pods[j]

		iIsMaster := NodeTypesMasterLabelName.HasValue(true, iPod.Labels)
		jIsMaster := NodeTypesMasterLabelName.HasValue(true, jPod.Labels)

		if iIsMaster {
			if jIsMaster {
				// both masters, so fall back to sorting by name
				return iPod.Name < jPod.Name
			} else {
				// i is master, j is not, so i should come first
				return true
			}
		}

		if jIsMaster {
			// i is not master, j is master, so j should come first
			return false
		}

		// neither are masters, sort by names
		return iPod.Name < jPod.Name
	}
}

// partitionPodsBySelector partitions pods into two sets: one for pods matching the selector and one for the rest. it
// guarantees that the order of the pods are not changed.
func partitionPodsBySelector(selector labels.Selector, remainingPods []corev1.Pod) ([]corev1.Pod, []corev1.Pod) {
	matchingPods := make([]corev1.Pod, 0)
	for i := len(remainingPods) - 1; i >= 0; i-- {
		pod := remainingPods[i]

		podLabels := labels.Set(pod.Labels)
		if selector.Matches(podLabels) {
			matchingPods = append(matchingPods, pod)

			remainingPods = append(remainingPods[:i], remainingPods[i+1:]...)
		}
	}

	// reverse the selected pods slice because our reverse order-iteration above reversed it
	for i := len(matchingPods)/2 - 1; i >= 0; i-- {
		opp := len(matchingPods) - 1 - i
		matchingPods[i], matchingPods[opp] = matchingPods[opp], matchingPods[i]
	}

	return matchingPods, remainingPods
}

// PerformableChanges contains changes that can be performed to pod resources
type PerformableChanges struct {
	// ScheduleForCreation are pods that can be created
	ScheduleForCreation []CreatablePod
	// ScheduleForDeletion are pods that can start the deletion process
	ScheduleForDeletion []corev1.Pod

	// MaxSurgeGroups are groups that hit their max surge.
	MaxSurgeGroups []int
	// MaxUnavailableGroups are groups that hit their max unavailable number.
	MaxUnavailableGroups []int
}

// IsEmpty is true if there are no changes.
func (c PerformableChanges) IsEmpty() bool {
	return len(c.ScheduleForCreation) == 0 && len(c.ScheduleForDeletion) == 0
}

// CreatablePod contains all information required to create a pod
type CreatablePod struct {
	Pod            corev1.Pod
	PodSpecContext PodSpecContext
}

// CalculatePerformableChanges calculates the PerformableChanges based on the groups and their associated strategies
func (s GroupedChangeSets) CalculatePerformableChanges() (*PerformableChanges, error) {
	var result PerformableChanges

	for groupIndex, groupedChangeSet := range s {
		keyNumbers := groupedChangeSet.KeyNumbers()

		log.Info(
			"Calculating performable changes for group",
			"group_index", groupIndex,
			"key_numbers", keyNumbers,
			"pods_state_status", groupedChangeSet.PodsState.Status(),
		)

		log.V(4).Info(
			"Calculating performable changes for group",
			"group_index", groupIndex,
			"pods_state_summary", groupedChangeSet.PodsState.Summary(),
		)

		// TODO: MaxUnavailable and MaxSurge would be great to have as intstrs, but due to
		// https://github.com/kubernetes-sigs/kubebuilder/issues/442 this is not currently an option.
		maxSurge := groupedChangeSet.Definition.Strategy.MaxSurge
		//maxSurge, err := intstr.GetValueFromIntOrPercent(
		//	&groupedChangeSet.Definition.Strategy.MaxSurge,
		//	targetPodsCount,
		//	true,
		//)
		//if err != nil {
		//	return nil, err
		//}

		maxUnavailable := groupedChangeSet.Definition.Strategy.MaxUnavailable
		//maxUnavailable, err := intstr.GetValueFromIntOrPercent(
		//	&groupedChangeSet.Definition.Strategy.MaxUnavailable,
		//	targetPodsCount,
		//	false,
		//)
		//if err != nil {
		//	return nil, err
		//}

		// schedule for creation as many pods as we can
		for _, newPodToAdd := range groupedChangeSet.ChangeSet.ToAdd {
			if keyNumbers.CurrentSurge >= maxSurge {
				msg := fmt.Sprintf(""+
					"Hit the max surge limit in group %d, currently growing at a rate of %d with %d current pods",
					groupIndex,
					keyNumbers.CurrentSurge,
					keyNumbers.CurrentPods,
				)
				log.Info(msg)
				result.MaxSurgeGroups = append(result.MaxSurgeGroups, groupIndex)
				break
			}

			keyNumbers.CurrentSurge++
			keyNumbers.CurrentPods++

			msg := fmt.Sprintf(
				"Adding a node in group %d, rate will now be %d with %d current pods.",
				groupIndex,
				keyNumbers.CurrentSurge,
				keyNumbers.CurrentPods,
			)
			log.Info(msg)

			toAddContext := groupedChangeSet.ChangeSet.ToAddContext[newPodToAdd.Name]

			log.Info(fmt.Sprintf(
				"Need to add pod because of the following mismatch reasons: %v",
				toAddContext.MismatchReasons,
			))

			result.ScheduleForCreation = append(
				result.ScheduleForCreation,
				CreatablePod{Pod: newPodToAdd, PodSpecContext: toAddContext.PodSpecCtx},
			)
		}

		// schedule for deletion as many pods as we can
		for _, pod := range groupedChangeSet.ChangeSet.ToRemove {
			// TODO: consider allowing removal of a pod if MaxUnavailable = 0 if all pods are operational?
			if keyNumbers.CurrentUnavailable >= maxUnavailable {
				msg := fmt.Sprintf(""+
					"Hit the max unavailable limit in group %d, currently shrinking with an unavailability of %d with %d current operational pods",
					groupIndex,
					keyNumbers.CurrentUnavailable,
					keyNumbers.CurrentOperationalPods,
				)
				log.Info(msg)
				result.MaxUnavailableGroups = append(result.MaxUnavailableGroups, groupIndex)
				break
			}

			keyNumbers.CurrentUnavailable++
			keyNumbers.CurrentOperationalPods--

			msg := fmt.Sprintf(
				"Removing a node in group %d, unavailability will now be %d with %d current operational pods.",
				groupIndex,
				keyNumbers.CurrentUnavailable,
				keyNumbers.CurrentOperationalPods,
			)
			log.Info(msg)

			result.ScheduleForDeletion = append(result.ScheduleForDeletion, pod)
		}
	}

	return &result, nil
}
