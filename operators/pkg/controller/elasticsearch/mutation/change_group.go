// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"fmt"
	"sort"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

const (
	// AllGroupName is the name used in ChangeGroups that is used for
	// changes that have not been partitioned into groups
	AllGroupName = "all"

	// UnmatchedGroupName is the name used in ChangeGroups for
	// a group that was not selected by the user-specified groups
	UnmatchedGroupName = "unmatched"

	// indexedGroupNamePrefix is the prefix used for dynamically named ChangeGroups
	indexedGroupNamePrefix = "group-"
)

// empty is used internally when referring to an empty struct instance
var empty struct{}

// indexedGroupName returns the group name to use for the given indexed group
func indexedGroupName(index int) string {
	return fmt.Sprintf("%s%d", indexedGroupNamePrefix, index)
}

// ChangeGroup holds changes for a specific group of pods
type ChangeGroup struct {
	// Name is a logical name for these changes
	Name string
	// Changes contains the changes in this group
	Changes Changes
	// PodsState contains the state of all the pods in this group
	PodsState PodsState
}

// ChangeStats contains key numbers for a ChangeGroup, used to execute an upgrade budget
type ChangeStats struct {
	// TargetPods is the number of pods we should have in the final state
	TargetPods int `json:"targetPods"`
	// CurrentPods is the current number of pods in the cluster that might be using resources
	CurrentPods int `json:"currentPods"`
	// CurrentSurge is the number of pods above the target the cluster is using
	CurrentSurge int `json:"currentSurge"`
	// CurrentRunningReadyPods is the number of pods that are running and have joined the current master
	CurrentRunningReadyPods int `json:"currentRunningReady"`
	// CurrentUnavailable is the number of pods below the target the cluster is currently using
	CurrentUnavailable int `json:"currentUnavailable"`
}

// ChangeStats calculates and returns the ChangeStats for this ChangeGroup
func (s ChangeGroup) ChangeStats() ChangeStats {
	// when we're done, we should have ToKeep + ToCreate pods in the group.
	targetPodsCount := len(s.Changes.ToKeep) + len(s.Changes.ToCreate)

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
func (s ChangeGroup) calculatePerformableChanges(
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

	// ensure we consider removing terminal pods first and the master node last in these changes
	sort.SliceStable(
		s.Changes.ToDelete,
		sortPodsByTerminalFirstMasterNodeLastAndCreationTimestampAsc(
			s.PodsState.Terminal,
			s.PodsState.MasterNodePod,
			s.Changes.ToDelete,
		),
	)

	// ensure we create master nodes first in this group
	sort.SliceStable(
		s.Changes.ToCreate,
		sortPodsToCreateByMasterNodesFirstThenNameAsc(s.Changes.ToCreate),
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
	for _, newPodToCreate := range s.Changes.ToCreate {
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

		log.V(4).Info(
			"Scheduling a pod for creation",
			"group_name", s.Name,
			"change_stats", changeStats,
			"mismatch_reasons", newPodToCreate.MismatchReasons,
		)

		result.ToCreate = append(result.ToCreate, newPodToCreate)
	}

	// schedule for deletion as many pods as we can
	for _, pod := range s.Changes.ToDelete {
		if _, ok := s.PodsState.Terminal[pod.Name]; ok {
			// removing terminal pods do not affect our availability budget, so we can always delete
			result.ToDelete = append(result.ToDelete, pod)
			continue
		}

		if err := podRestrictions.CanDelete(pod); err != nil {
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
		result.ToDelete = append(result.ToDelete, pod)
	}

	return nil
}

// simulatePerformableChangesApplied applies the performable changes to the ChangeGroup
func (s *ChangeGroup) simulatePerformableChangesApplied(
	performableChanges PerformableChanges,
) {
	// convert the scheduled for deletion pods to a map for faster lookup
	toDeleteByName := make(map[string]struct{}, len(performableChanges.ToDelete))
	for _, pod := range performableChanges.ToDelete {
		toDeleteByName[pod.Name] = empty
	}

	// for each pod we intend to remove, simulate a deletion
	for i := len(s.Changes.ToDelete) - 1; i >= 0; i-- {
		podToDelete := s.Changes.ToDelete[i]
		if _, ok := toDeleteByName[podToDelete.Name]; ok {
			// pop from list of pods to delete
			s.Changes.ToDelete = append(s.Changes.ToDelete[:i], s.Changes.ToDelete[i+1:]...)
		}
	}

	// convert the scheduled for creation pods to a map for faster lookup
	toCreateByName := make(map[string]struct{}, len(performableChanges.ToCreate))
	for _, podToCreate := range performableChanges.ToCreate {
		toCreateByName[podToCreate.Pod.Name] = empty
	}

	// for each pod we intend to create, simulate the creation
	for i := len(s.Changes.ToCreate) - 1; i >= 0; i-- {
		podToCreate := s.Changes.ToCreate[i]
		if _, ok := toCreateByName[podToCreate.Pod.Name]; ok {
			// pop from list of pods to create
			s.Changes.ToCreate = append(s.Changes.ToCreate[:i], s.Changes.ToCreate[i+1:]...)
			// pretend we created it, which would move it to Pending
			s.PodsState.Pending[podToCreate.Pod.Name] = podToCreate.Pod
			// also pretend we're intending to keep it instead of creating it
			s.Changes.ToKeep = append(s.Changes.ToKeep, podToCreate.Pod)
		}
	}

	var remaining PodsState
	// update the current pod states to match the simulated changes
	s.PodsState, remaining = s.PodsState.Partition(s.Changes)
	// The partition above removes any pods not part of the .Changes from the PodsState, which includes pods that have
	// been deleted by an external process or another reconciliation iteration. These should still exist in the
	// simulated PodsState, so we need to add these back in specifically.
	for _, pod := range remaining.Deleting {
		s.PodsState.Deleting[pod.Name] = pod
	}

	// deleted pods should eventually go into a Deleting state,
	// simulate that for deleted pods to be counted as unavailable
	for _, pod := range performableChanges.ToDelete {
		s.PodsState.Deleting[pod.Name] = pod
	}
}

// ChangeGroups is a list of ChangeGroup
type ChangeGroups []ChangeGroup

// calculatePerformableChanges calculates the PerformableChanges for each group with the given budget
func (s ChangeGroups) calculatePerformableChanges(
	budget v1alpha1.ChangeBudget,
	podRestrictions *PodRestrictions,
	result *PerformableChanges,
) error {
	for _, group := range s {
		if err := group.calculatePerformableChanges(budget, podRestrictions, result); err != nil {
			return err
		}
	}

	return nil
}
