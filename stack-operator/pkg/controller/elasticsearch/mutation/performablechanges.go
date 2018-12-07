package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
)

var (
	// pass1ChangeBudget is very restrictive change budget used for the first pass when calculating performable changes
	pass1ChangeBudget = v1alpha1.ChangeBudget{}
)

// PerformableChanges contains changes that can be performed to pod resources
type PerformableChanges struct {
	// ScheduleForCreation are pods that can be created
	ScheduleForCreation []CreatablePod
	// ScheduleForDeletion are pods that can start the deletion process
	ScheduleForDeletion []corev1.Pod

	// informational values
	// RestrictedPods are pods that were prevented from being scheduled for deletion
	RestrictedPods map[string]error
	// MaxSurgeGroups are groups that hit their max surge.
	MaxSurgeGroups []string
	// MaxUnavailableGroups are groups that hit their max unavailable number.
	MaxUnavailableGroups []string
}

// HasChanges is true if there are no changes.
func (c PerformableChanges) HasChanges() bool {
	return len(c.ScheduleForCreation) > 0 || len(c.ScheduleForDeletion) > 0
}

// CreatablePod contains all information required to create a pod
type CreatablePod struct {
	Pod            corev1.Pod
	PodSpecContext support.PodSpecContext
}

// initializePerformableChanges initializes nil values in PerformableChanges
func initializePerformableChanges(changes PerformableChanges) PerformableChanges {
	if changes.RestrictedPods == nil {
		changes.RestrictedPods = make(map[string]error)
	}
	return changes
}

// CalculatePerformableChanges calculates which changes that can be performed in the current state.
func CalculatePerformableChanges(
	strategy v1alpha1.UpdateStrategy,
	allPodChanges *ChangeSet,
	allPodsState PodsState,
) (*PerformableChanges, error) {
	performableChanges := initializePerformableChanges(PerformableChanges{})

	// resolve the change budget
	budget := strategy.ResolveChangeBudget()

	// allChangeSet is a GroupedChangeSet that contains all the changes in a single group
	allChangeSet := GroupedChangeSet{
		Name:      AllGroupName,
		ChangeSet: *allPodChanges,
		PodsState: allPodsState,
	}

	// group all our changes into groups based on the potentially user-specified groups
	groupedChangeSets, err := allPodChanges.Group(strategy.Groups, allPodsState)
	if err != nil {
		return nil, err
	}
	log.V(3).Info("Created grouped change sets", "count", len(groupedChangeSets))

	podRestrictions := NewPodRestrictions(allPodsState)

	// pass 1:
	// - give every group a chance to perform changes, but do not allow for any surge or unavailability. this is
	// intended to ensure that we're able to recover from larger failures (e.g a pod failing or a failure domain
	// falling apart). this is to ensure that the surge/unavailability room that's created by the failing pods do not
	// get eaten up other, simultaneous changes.
	if err := groupedChangeSets.calculatePerformableChanges(
		pass1ChangeBudget,
		&podRestrictions,
		&performableChanges,
	); err != nil {
		return nil, err
	}

	// apply the performable changes to the "all" (ungrouped) changeset. this is done in order to account for the
	// changes pass 1 is intending to do.
	allChangeSet.simulatePerformableChangesApplied(performableChanges)

	// pass 2:
	// - calculate the performable changes across a single changeset using the proper budget.
	if err := allChangeSet.calculatePerformableChanges(
		budget,
		&podRestrictions,
		&performableChanges,
	); err != nil {
		return nil, err
	}

	// pass 3:
	// - in which we allow breaking the surge budget if we have changes we would like to apply, but were not allowed to
	// due to the surge budget
	// - this is required for scenarios such as converting from one MasterData node to one Master and One Data. In
	// this situation we *must* create both new nodes before we delete the existing one
	// TODO: consider requiring this being enabled in the update strategy?
	// TODO: determine whether restricted changes from podRestrictions where a cluster is temporarily unavailable can
	// cause this to be a bit too permissive.

	if !allChangeSet.ChangeSet.IsEmpty() && !performableChanges.HasChanges() {
		changeStats := allChangeSet.ChangeStats()
		newBudget := v1alpha1.ChangeBudget{
			MaxSurge: changeStats.CurrentSurge + 1,
		}

		// - here we do not have to simulate performing changes because we know it has no changes

		if err := allChangeSet.calculatePerformableChanges(
			newBudget,
			&podRestrictions,
			&performableChanges,
		); err != nil {
			return nil, err
		}
	}

	return &performableChanges, nil
}
