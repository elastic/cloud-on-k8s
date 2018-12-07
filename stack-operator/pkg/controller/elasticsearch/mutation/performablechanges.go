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

	// MaxSurgeGroups are groups that hit their max surge.
	MaxSurgeGroups []string
	// MaxUnavailableGroups are groups that hit their max unavailable number.
	MaxUnavailableGroups []string
}

// IsEmpty is true if there are no changes.
func (c PerformableChanges) IsEmpty() bool {
	return len(c.ScheduleForCreation) == 0 && len(c.ScheduleForDeletion) == 0
}

// CreatablePod contains all information required to create a pod
type CreatablePod struct {
	Pod            corev1.Pod
	PodSpecContext support.PodSpecContext
}

// CalculatePerformableChanges calculates which changes we are allowed to perform in the current state.
func CalculatePerformableChanges(
	strategy v1alpha1.UpdateStrategy,
	allPodChanges *ChangeSet,
	allPodsState PodsState,
) (*PerformableChanges, error) {
	performableChanges := &PerformableChanges{}

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
	log.Info("Created grouped change sets", "count", len(groupedChangeSets))

	// pass 1:
	// - give every group a change to perform changes, but do not allow for any surge or unavailability. this is
	// intended to ensure that we're able to recover from larger failures (e.g a pod failing or a failure domain
	// falling apart). this is to ensure that the surge/unavailability room that's created by the failing pods do not
	// get eaten up other, simultaneous changes.
	if err := groupedChangeSets.calculatePerformableChanges(
		pass1ChangeBudget,
		performableChanges,
	); err != nil {
		return nil, err
	}

	// apply the performable changes to the "all" (ungrouped) changeset. this is done in order to account for the
	// changes pass 1 is intending to do.
	allChangeSet.simulatePerformableChangesApplied(*performableChanges)

	// pass 2:
	// - calculate the performable changes across a single changeset using the proper budget.
	if err := allChangeSet.calculatePerformableChanges(budget, performableChanges); err != nil {
		return nil, err
	}

	return performableChanges, nil
}
