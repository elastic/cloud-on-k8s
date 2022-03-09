// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package events

// Event reasons for the Elastic stack controller
const (
	// EventReasonDelayed describes events where a requested change was delayed e.g. to prevent data loss.
	EventReasonDelayed = "Delayed"
	// EventReasonStalled describes events where a requested change is stalled and may not make progress without user
	// intervention. There are transient states e.g. during a nodeSet rename where shards still do not have a place to
	// move to until the new nodes come up and Elasticsearch will report a stalled shutdown. There are however also
	// permanent states if the new topology requested by the user does not have enough space for the shards which requires
	// user intervention to correct the mistake.
	EventReasonStalled = "Stalled"
	// EventReasonUpgraded describes events where resources are upgraded.
	EventReasonUpgraded = "Upgraded"
	// EventReasonUnhealthy describes events where a stack deployments health was affected negatively.
	EventReasonUnhealthy = "Unhealthy"
	// EventReasonUnexpected describes events that were not anticipated or happened at an unexpected time.
	EventReasonUnexpected = "Unexpected"
	// EventReasonValidation describes events that were due to an invalid resource being submitted by the user.
	EventReasonValidation = "Validation"
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
