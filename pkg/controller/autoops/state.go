// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
)

// State holds the accumulated state during the reconcile loop including the response and a copy of the
// AutoOpsAgentPolicy resource from the start of reconciliation, for status updates.
type State struct {
	*events.Recorder
	policy autoopsv1alpha1.AutoOpsAgentPolicy
	status autoopsv1alpha1.AutoOpsAgentPolicyStatus
}

// newState creates a new reconcile state based on the given policy
func newState(policy autoopsv1alpha1.AutoOpsAgentPolicy) *State {
	status := *policy.Status.DeepCopy()
	status.ObservedGeneration = policy.Generation
	// Similar to ES, we initially set the phase to an empty string so that we do not report an outdated phase
	// given that certain phases are stickier than others (eg. invalid)
	status.Phase = ""
	status.Errors = 0
	status.Ready = 0
	status.Resources = 0
	status.Skipped = 0
	// Initialize Details map to avoid nil map assignment panics
	status.Details = make(map[string]autoopsv1alpha1.AutoOpsResourceStatus)
	return &State{
		Recorder: events.NewRecorder(),
		policy:   policy,
		status:   status,
	}
}

// UpdateWithPhase updates the phase of the AutoOpsAgentPolicy status.
// It respects phase stickiness - InvalidPhase and NoResourcesPhase will not be overwritten,
// and ApplyingChangesPhase and ReadyPhase will not overwrite other non-ready phases.
func (s *State) UpdateWithPhase(phase autoopsv1alpha1.PolicyPhase) *State {
	// Only update if new phase is "worse" (higher priority number)
	// InvalidPhase is terminal and never changes
	if phase.Priority() >= s.status.Phase.Priority() {
		s.status.Phase = phase
	}

	return s
}

// UpdateInvalidPhaseWithEvent is a convenient method to set the phase to InvalidPhase
// and generate an event at the same time.
func (s *State) UpdateInvalidPhaseWithEvent(msg string) {
	s.UpdateWithPhase(autoopsv1alpha1.InvalidPhase)
	s.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, msg)
}

// UpdateMonitoredResources updates the Resources count in the status.
// If the count is zero it will try to apply [autoopsv1alpha1.NoMonitoredResourcesPhase] and it is solely responsible for applying this phase.
func (s *State) UpdateMonitoredResources(count int) *State {
	s.status.Resources = count
	if count == 0 {
		s.UpdateWithPhase(autoopsv1alpha1.NoMonitoredResourcesPhase)
	}
	return s
}

// MarkResourceReady increases the Ready count in the status.
func (s *State) MarkResourceReady() *State {
	s.status.Ready++
	return s
}

// Apply takes the current AutoOpsAgentPolicy status, compares it to the previous status, and updates the status accordingly.
// It returns the events to emit and an updated version of the AutoOpsAgentPolicy resource with
// the current status applied to its status sub-resource.
func (s *State) Apply() ([]events.Event, *autoopsv1alpha1.AutoOpsAgentPolicy) {
	previous := s.policy.Status
	current := s.status
	if reflect.DeepEqual(previous, current) {
		return s.Events(), nil
	}
	s.policy.Status = current
	return s.Events(), &s.policy
}

func (s *State) ResourceSkippedDueToRBAC(es esv1.Elasticsearch) {
	s.ResourceSkipped(es, fmt.Sprintf("RBAC access denied for service account %s", s.policy.Spec.ServiceAccountName))
}

func (s *State) ResourceSkippedDueToVersion(es esv1.Elasticsearch) {
	s.ResourceSkipped(es, fmt.Sprintf("ES cluster is in deprecated version %s", es.Spec.Version))
}

func (s *State) ResourceSkipped(es esv1.Elasticsearch, message string) {
	s.status.Skipped++
	s.status.Details[s.esResourceID(es)] = autoopsv1alpha1.AutoOpsResourceStatus{
		Phase:   autoopsv1alpha1.SkippedResourcePhase,
		Message: message,
	}
}

func (s *State) ResourceError(es esv1.Elasticsearch, message string, err error) {
	s.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
	s.status.Errors++
	s.status.Details[s.esResourceID(es)] = autoopsv1alpha1.AutoOpsResourceStatus{
		Phase: autoopsv1alpha1.ErrorResourcePhase,
		Error: fmt.Sprintf("%s: %s", message, err),
	}
}

func (s *State) esResourceID(es esv1.Elasticsearch) string {
	return fmt.Sprintf("%s/%s", es.Namespace, es.Name)
}

// Finalize updates the phase of the AutoOpsAgentPolicy status based on the results of the reconciliation and sets the status message and the ready count.
// This method is solely responsible for applying the [autoopsv1alpha1.ApplyingChangesPhase], [autoopsv1alpha1.AutoOpsAgentsNotReadyPhase] and [autoopsv1alpha1.ReadyPhase].
func (s *State) Finalize(isReconciled bool, reconciliationMessage string) {
	switch {
	case !isReconciled:
		s.UpdateWithPhase(autoopsv1alpha1.ApplyingChangesPhase)
		s.AddEvent(corev1.EventTypeWarning, events.EventReasonDelayed, reconciliationMessage)
	case s.status.Ready == s.status.Resources:
		s.UpdateWithPhase(autoopsv1alpha1.ReadyPhase)
	case s.status.Ready < s.status.Resources:
		s.UpdateWithPhase(autoopsv1alpha1.AutoOpsAgentsNotReadyPhase)
	}

	s.status.ReadyCount = fmt.Sprintf("%d/%d", s.status.Ready, s.status.Resources)
}
