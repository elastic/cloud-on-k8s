package mutation

import (
	"sort"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
)

const (
	// AllGroupName is the name used in GroupedChangeSets that is used for changes that have not been partitioned into
	// groups
	AllGroupName = "all"

	// UnmatchedGroupName is the name used in GroupedChangeSets for a group that were not selected by the user-specified
	// groups
	UnmatchedGroupName = "unmatched"

	// DynamicGroupNamePrefix is the prefix used for dynamically named GroupedChangeSets.
	DynamicGroupNamePrefix = "group-"
)

// GroupedChangeSet is a ChangeSet for a specific group of pods.
type GroupedChangeSet struct {
	// Name is a logical name for these changes.
	Name string
	// ChangeSet contains the changes in this group
	ChangeSet ChangeSet
	// PodsState contains the state of all the pods in this group.
	PodsState PodsState
}

// KeyNumbers contains key numbers for a GroupedChangeSet, used to execute an upgrade budget
type KeyNumbers struct {
	// TargetPods is the number of pods we should have in the final state.
	TargetPods int `json:"targetPods"`
	// CurrentPods is the current number of pods in the cluster that might be using resources.
	CurrentPods int `json:"currentPods"`
	// CurrentSurge is the number of pods above the target the cluster is using.
	CurrentSurge int `json:"currentSurge"`
	// CurrentRunningReadyPods is the number of pods that are running and have joined the current master.
	CurrentRunningReadyPods int `json:"currentRunningReady"`
	// CurrentUnavailable is the number of pods below the target the cluster is currently using.
	CurrentUnavailable int `json:"currentUnavailable"`
}

// KeyNumbers calculates and returns the KeyNumbers for this grouped change set,
func (s GroupedChangeSet) KeyNumbers() KeyNumbers {
	// when we're done, we should have ToKeep + ToAdd pods in the group.
	targetPodsCount := len(s.ChangeSet.ToKeep) + len(s.ChangeSet.ToAdd)

	currentPodsCount := s.PodsState.CurrentPodsCount()

	// surge is the number of pods potentially consuming any resources we currently have above the target
	currentSurge := currentPodsCount - targetPodsCount

	currentRunningReadyPods := len(s.PodsState.RunningReady)

	// unavailable is the number of "running and ready" pods we have below the target
	currentUnavailable := targetPodsCount - currentRunningReadyPods

	return KeyNumbers{
		TargetPods:              targetPodsCount,
		CurrentPods:             currentPodsCount,
		CurrentSurge:            currentSurge,
		CurrentRunningReadyPods: currentRunningReadyPods,
		CurrentUnavailable:      currentUnavailable,
	}
}

// calculatePerformableChanges calculates the PerformableChanges for this group with the given budget
func (s GroupedChangeSet) calculatePerformableChanges(
	budget v1alpha1.ChangeBudget,
	result *PerformableChanges,
) error {
	keyNumbers := s.KeyNumbers()

	log.Info(
		"Calculating performable changes for group",
		"group_name", s.Name,
		"key_numbers", keyNumbers,
		"pods_state_status", s.PodsState.Status(),
	)

	log.V(4).Info(
		"Calculating performable changes for group",
		"group_name", s.Name,
		"pods_state_summary", s.PodsState.Summary(),
	)

	// TODO: the sorting done here does not guarantee that we have both master and data nodes available in the cluster
	// at all times.

	// ensure we remove the master node last in this changeset
	sort.SliceStable(
		s.ChangeSet.ToRemove,
		sortPodsByMasterNodeLastAndCreationTimestampAsc(
			s.PodsState.MasterNodePod,
			s.ChangeSet.ToRemove,
		),
	)

	// ensure we add master nodes first in this changeset
	sort.SliceStable(
		s.ChangeSet.ToAdd,
		sortPodsByMasterNodesFirstThenNameAsc(s.ChangeSet.ToAdd),
	)

	// TODO: MaxUnavailable and MaxSurge would be great to have as intstrs, but due to
	// https://github.com/kubernetes-sigs/kubebuilder/issues/442 this is not currently an option.
	maxSurge := budget.MaxSurge
	//maxSurge, err := intstr.GetValueFromIntOrPercent(
	//	&s.Definition.ChangeBudget.MaxSurge,
	//	targetPodsCount,
	//	true,
	//)
	//if err != nil {
	//	return err
	//}

	maxUnavailable := budget.MaxUnavailable
	//maxUnavailable, err := intstr.GetValueFromIntOrPercent(
	//	&s.Definition.ChangeBudget.MaxUnavailable,
	//	targetPodsCount,
	//	false,
	//)
	//if err != nil {
	//	return err
	//}

	// schedule for creation as many pods as we can
	for _, newPodToAdd := range s.ChangeSet.ToAdd {
		if keyNumbers.CurrentSurge >= maxSurge {
			log.Info(
				"Hit the max surge limit in a group.",
				"group_name", s.Name,
				"key_numbers", keyNumbers,
			)
			result.MaxSurgeGroups = append(result.MaxSurgeGroups, s.Name)
			break
		}

		keyNumbers.CurrentSurge++
		keyNumbers.CurrentPods++

		toAddContext := s.ChangeSet.ToAddContext[newPodToAdd.Name]

		log.Info(
			"Scheduling a pod for creation",
			"group_name", s.Name,
			"key_numbers", keyNumbers,
			"mismatch_reasons", toAddContext.MismatchReasons,
		)

		result.ScheduleForCreation = append(
			result.ScheduleForCreation,
			CreatablePod{Pod: newPodToAdd, PodSpecContext: toAddContext.PodSpecCtx},
		)
	}

	// schedule for deletion as many pods as we can
	for _, pod := range s.ChangeSet.ToRemove {
		if keyNumbers.CurrentUnavailable >= maxUnavailable {
			log.Info(
				"Hit the max unavailable limit in a group.",
				"group_name", s.Name,
				"key_numbers", keyNumbers,
			)

			result.MaxUnavailableGroups = append(result.MaxUnavailableGroups, s.Name)
			break
		}

		keyNumbers.CurrentUnavailable++
		keyNumbers.CurrentRunningReadyPods--

		log.Info(
			"Scheduling a pod for deletion",
			"group_name", s.Name,
			"key_numbers", keyNumbers,
		)

		result.ScheduleForDeletion = append(result.ScheduleForDeletion, pod)
	}

	return nil
}

// applyPerformableChanges applies the performable changes to the GroupedChangeSet
func (s *GroupedChangeSet) applyPerformableChanges(
	performableChanges PerformableChanges,
) {
	// convert the scheduled for deletion pods to a map for faster lookup
	scheduledForDeletionByName := make(map[string]bool, len(performableChanges.ScheduleForDeletion))
	for _, pod := range performableChanges.ScheduleForDeletion {
		scheduledForDeletionByName[pod.Name] = true
	}

	// for each pod we intend to remove, if it was scheduled for deletion, pop it from ToRemove
	for i := len(s.ChangeSet.ToRemove) - 1; i >= 0; i-- {
		if scheduledForDeletionByName[s.ChangeSet.ToRemove[i].Name] {
			s.ChangeSet.ToRemove = append(s.ChangeSet.ToRemove[:i], s.ChangeSet.ToRemove[i+1:]...)
		}
	}

	// convert the scheduled for creation pods to a map for faster lookup
	handledAddPods := make(map[string]bool, len(performableChanges.ScheduleForCreation))
	for _, podToCreate := range performableChanges.ScheduleForCreation {
		handledAddPods[podToCreate.Pod.Name] = true

		// pretend we added it, which would move it to Pending
		s.PodsState.Pending[podToCreate.Pod.Name] = podToCreate.Pod
		// also pretend we're intending to keep it instead of adding it.
		s.ChangeSet.ToKeep = append(s.ChangeSet.ToKeep, podToCreate.Pod)
	}

	// for each pod we intend to add, if it was scheduled for creation, pop it from ToAdd
	for i := len(s.ChangeSet.ToAdd) - 1; i >= 0; i-- {
		if handledAddPods[s.ChangeSet.ToAdd[i].Name] {
			s.ChangeSet.ToAdd = append(s.ChangeSet.ToAdd[:i], s.ChangeSet.ToAdd[i+1:]...)
		}
	}

	s.PodsState, _ = s.PodsState.Partition(s.ChangeSet)
}

// GroupedChangeSets is a list of GroupedChangeSets
type GroupedChangeSets []GroupedChangeSet

// calculatePerformableChanges calculates the PerformableChanges for each group with the given budget
func (s GroupedChangeSets) calculatePerformableChanges(
	budget v1alpha1.ChangeBudget,
	result *PerformableChanges,
) error {
	for _, groupedChangeSet := range s {
		if err := groupedChangeSet.calculatePerformableChanges(budget, result); err != nil {
			return err
		}
	}

	return nil
}
