// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package events

// Event reasons for the Elastic stack controller
const (
	// EventReasonCreated describes events where resources were created.
	EventReasonCreated = "Created"
	// EventReasonDeleted describes events where resources were deleted.
	EventReasonDeleted = "Deleted"
	// EventReasonDelayed describes events where a requested change was delayed e.g. to prevent data loss.
	EventReasonDelayed = "Delayed"
	// EventReasonCreated describes events where resources are upgraded.
	EventReasonUpgraded = "Upgraded"
	// EventReasonUnhealthy describes events where a stack deployments health was affected negatively.
	EventReasonUnhealthy = "Unhealthy"
	// EventReasonUnexpected describes events that were not anticipated or happened at an unexpected time.
	EventReasonUnexpected = "Unexpected"
	// EventReasonValidation describes events that were due to an invalid resource being submitted by the user.
	EventReasonValidation = "Validation"
	// EventReasonStateChange describes events that are expected state changes in a Elasticsearch cluster.
	EventReasonStateChange = "StateChange"
	// EventReasonRestart describes events where one or multiple Elasticsearch nodes are scheduled for a restart.
	EventReasonRestart = "Restart"
)

// Event reasons for Association controllers
const (
	// EventAssociationError describes an event fired when an association fails.
	EventAssociationError = "AssociationError"
	// EventAssociationStatusChange describes association status change events.
	EventAssociationStatusChange = "AssociationStatusChange"
)

// Event reasons for common error conditions
const (
	// EventReconciliationError describes an error detected during reconciliation of an object.
	EventReconciliationError = "ReconciliationError"
	// EventCompatCheckError describes an error during the check for compatibility between operator version and managed resources.
	EventCompatCheckError = "CompatibilityCheckError"
)

// Event is a k8s event that can be recorded via an event recorder.
type Event struct {
	EventType string
	Reason    string
	Message   string
}

// Recorder keeps track of events.
type Recorder struct {
	events []Event
}

// NewRecorder returns an initialized event recorder.
func NewRecorder() *Recorder {
	return &Recorder{events: []Event{}}
}

// AddEvent records the intent to emit a k8s event with the given attributes.
func (r *Recorder) AddEvent(eventType, reason, message string) {
	if r.events == nil {
		r.events = []Event{}
	}
	r.events = append(r.events, Event{
		eventType,
		reason,
		message,
	})
}

// Events returns all recorded events.
func (r *Recorder) Events() []Event {
	return r.events
}
