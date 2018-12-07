package mutation

import (
	"fmt"
	"sort"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
)

const (
	// AllGroupName is the name used in GroupedChangeSets that is used for changes that have not been partitioned into
	// groups
	AllGroupName = "all"

	// UnmatchedGroupName is the name used in GroupedChangeSets for a group that was not selected by the user-specified
	// groups
	UnmatchedGroupName = "unmatched"

	// indexedGroupNamePrefix is the prefix used for dynamically named GroupedChangeSets.
	indexedGroupNamePrefix = "group-"
)

// empty is used internally when referring to an empty struct instance
var empty struct{}

// indexedGroupName returns the group name to use for the given indexed group
func indexedGroupName(index int) string {
	return fmt.Sprintf("%s%d", indexedGroupNamePrefix, index)
}

// GroupedChangeSet is a ChangeSet for a specific group of pods.
type GroupedChangeSet struct {
	// Name is a logical name for these changes.
	Name string
	// ChangeSet contains the changes in this group
	ChangeSet ChangeSet
	// PodsState contains the state of all the pods in this group.
	PodsState PodsState
}

// ChangeStats contains key numbers for a GroupedChangeSet, used to execute an upgrade budget
type ChangeStats struct {
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

// ChangeStats calculates and returns the ChangeStats for this grouped change set,
func (s GroupedChangeSet) ChangeStats() ChangeStats {
	// when we're done, we should have ToKeep + ToAdd pods in the group.
	targetPodsCount := len(s.ChangeSet.ToKeep) + len(s.ChangeSet.ToAdd)

	currentPodsCount := s.PodsState.CurrentPodsCount()

	// surge is the number of pods potentially consuming any resources we currently have above the target
	currentSurge := currentPodsCount - targetPodsCount

	currentRunningReadyPods := len(s.PodsState.RunningReady)

	// unavailable is the number of "running and ready" pods that are missing compared to the target, iow pods
	currentUnavailable := targetPodsCount - currentRunningReadyPods

	return ChangeStats{
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
	podRestrictions *PodRestrictions,
	result *PerformableChanges,
) error {
	changeStats := s.ChangeStats()

	log.V(3).Info(
		"Calculating performable changes for group",
		"group_name", s.Name,
		"change_stats", changeStats,
		"pods_state_status", s.PodsState.Status(),
	)

	log.V(4).Info(
		"Calculating performable changes for group",
		"group_name", s.Name,
		"pods_state_summary", s.PodsState.Summary(),
	)

	// ensure we consider removing terminal pods first and the master node last in this changeset
	sort.SliceStable(
		s.ChangeSet.ToRemove,
		sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc(
			s.PodsState.Terminal,
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
		if changeStats.CurrentSurge >= maxSurge {
			log.V(4).Info(
				"Hit the max surge limit in a group.",
				"group_name", s.Name,
				"change_stats", changeStats,
			)
			result.MaxSurgeGroups = append(result.MaxSurgeGroups, s.Name)
			break
		}

		changeStats.CurrentSurge++
		changeStats.CurrentPods++

		toAddContext := s.ChangeSet.ToAddContext[newPodToAdd.Name]

		log.V(4).Info(
			"Scheduling a pod for creation",
			"group_name", s.Name,
			"change_stats", changeStats,
			"mismatch_reasons", toAddContext.MismatchReasons,
		)

		result.ScheduleForCreation = append(
			result.ScheduleForCreation,
			CreatablePod{Pod: newPodToAdd, PodSpecContext: toAddContext.PodSpecCtx},
		)
	}

	// schedule for deletion as many pods as we can
	for _, pod := range s.ChangeSet.ToRemove {
		if _, ok := s.PodsState.Terminal[pod.Name]; ok {
			// removing terminal pods do not affect our availability budget, so we can always delete
			result.ScheduleForDeletion = append(result.ScheduleForDeletion, pod)
			continue
		}

		if err := podRestrictions.CanRemove(pod); err != nil {
			// cannot remove pod due to restrictions
			result.RestrictedPods[pod.Name] = err
			continue
		}

		if changeStats.CurrentUnavailable >= maxUnavailable {
			log.V(4).Info(
				"Hit the max unavailable limit in a group.",
				"group_name", s.Name,
				"change_stats", changeStats,
			)

			result.MaxUnavailableGroups = append(result.MaxUnavailableGroups, s.Name)
			break
		}

		changeStats.CurrentUnavailable++
		changeStats.CurrentRunningReadyPods--

		log.V(4).Info(
			"Scheduling a pod for deletion",
			"group_name", s.Name,
			"change_stats", changeStats,
		)

		podRestrictions.Remove(pod)
		result.ScheduleForDeletion = append(result.ScheduleForDeletion, pod)
	}

	return nil
}

// simulatePerformableChangesApplied applies the performable changes to the GroupedChangeSet
func (s *GroupedChangeSet) simulatePerformableChangesApplied(
	performableChanges PerformableChanges,
) {
	// convert the scheduled for deletion pods to a map for faster lookup
	scheduledForDeletionByName := make(map[string]struct{}, len(performableChanges.ScheduleForDeletion))
	for _, pod := range performableChanges.ScheduleForDeletion {
		scheduledForDeletionByName[pod.Name] = empty
	}

	// for each pod we intend to remove, if it was scheduled for deletion, pop it from ToRemove
	for i := len(s.ChangeSet.ToRemove) - 1; i >= 0; i-- {
		if _, ok := scheduledForDeletionByName[s.ChangeSet.ToRemove[i].Name]; ok {
			s.ChangeSet.ToRemove = append(s.ChangeSet.ToRemove[:i], s.ChangeSet.ToRemove[i+1:]...)
		}
	}

	// convert the scheduled for creation pods to a map for faster lookup
	scheduledForCreationByName := make(map[string]struct{}, len(performableChanges.ScheduleForCreation))
	for _, podToCreate := range performableChanges.ScheduleForCreation {
		scheduledForCreationByName[podToCreate.Pod.Name] = empty

		// pretend we added it, which would move it to Pending
		s.PodsState.Pending[podToCreate.Pod.Name] = podToCreate.Pod
		// also pretend we're intending to keep it instead of adding it.
		s.ChangeSet.ToKeep = append(s.ChangeSet.ToKeep, podToCreate.Pod)
		// remove from the to add context as it's being added
		delete(s.ChangeSet.ToAddContext, podToCreate.Pod.Name)
	}

	// for each pod we intend to add, if it was scheduled for creation, pop it from ToAdd
	for i := len(s.ChangeSet.ToAdd) - 1; i >= 0; i-- {
		if _, ok := scheduledForCreationByName[s.ChangeSet.ToAdd[i].Name]; ok {
			s.ChangeSet.ToAdd = append(s.ChangeSet.ToAdd[:i], s.ChangeSet.ToAdd[i+1:]...)
		}
	}

	// this leaves PodsState, which we can simply partition by the new changeset
	s.PodsState, _ = s.PodsState.Partition(s.ChangeSet)

	// removed pods will /eventually/ go to the Deleting stage, and since we're just removing it from the ChangeSet
	// above, we need to pretend it's being deleted for it to be counted as unavailable.
	for _, pod := range performableChanges.ScheduleForDeletion {
		s.PodsState.Deleting[pod.Name] = pod
	}
}

// GroupedChangeSets is a list GroupedChangeSet instances
type GroupedChangeSets []GroupedChangeSet

// calculatePerformableChanges calculates the PerformableChanges for each group with the given budget
func (s GroupedChangeSets) calculatePerformableChanges(
	budget v1alpha1.ChangeBudget,
	podRestrictions *PodRestrictions,
	result *PerformableChanges,
) error {
	for _, groupedChangeSet := range s {
		if err := groupedChangeSet.calculatePerformableChanges(budget, podRestrictions, result); err != nil {
			return err
		}
	}

	return nil
}
