// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// State holds the accumulated state during the reconcile loop including the response and a copy of the
// AutoOpsAgentPolicy resource from the start of reconciliation, for status updates.
type State struct {
	*events.Recorder
	policy autoopsv1alpha1.AutoOpsAgentPolicy
	status autoopsv1alpha1.AutoOpsAgentPolicyStatus
}

// NewState creates a new reconcile state based on the given policy
func NewState(policy autoopsv1alpha1.AutoOpsAgentPolicy) *State {
	status := *policy.Status.DeepCopy()
	status.ObservedGeneration = policy.Generation
	// Similar to ES, we initially set the phase to an empty string so that we do not report an outdated phase
	// given that certain phases are stickier than others (eg. invalid)
	status.Phase = ""
	return &State{
		Recorder: events.NewRecorder(),
		policy:   policy,
		status:   status,
	}
}

// UpdateWithPhase updates the phase of the AutoOpsAgentPolicy status.
// It respects phase stickiness - InvalidPhase will not be overwritten, and ApplyingChangesPhase
// and ReadyPhase will not overwrite other non-ready phases.
func (s *State) UpdateWithPhase(phase autoopsv1alpha1.PolicyPhase) *State {
	nonReadyPhases := set.Make(
		string(autoopsv1alpha1.ErrorPhase),
		string(autoopsv1alpha1.NoResourcesPhase),
		string(autoopsv1alpha1.UnknownPhase),
	)
	switch {
	// do not overwrite the Invalid phase
	case s.status.Phase == autoopsv1alpha1.InvalidPhase:
		return s
	// do not overwrite non-ready phases with ApplyingChangesPhase
	case phase == autoopsv1alpha1.ApplyingChangesPhase && nonReadyPhases.Has(string(s.status.Phase)):
		return s
	// do not overwrite non-ready phases with ReadyPhase
	case phase == autoopsv1alpha1.ReadyPhase && nonReadyPhases.Has(string(s.status.Phase)):
		return s
	}
	s.status.Phase = phase
	return s
}

// UpdateInvalidPhaseWithEvent is a convenient method to set the phase to InvalidPhase
// and generate an event at the same time.
func (s *State) UpdateInvalidPhaseWithEvent(msg string) {
	s.status.Phase = autoopsv1alpha1.InvalidPhase
	s.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, msg)
}

// UpdateResources updates the Resources count in the status.
func (s *State) UpdateResources(count int) *State {
	s.status.Resources = count
	return s
}

// UpdateReady updates the Ready count in the status.
func (s *State) UpdateReady(count int) *State {
	s.status.Ready = count
	return s
}

// UpdateErrors updates the Errors count in the status.
func (s *State) UpdateErrors(count int) *State {
	s.status.Errors = count
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
