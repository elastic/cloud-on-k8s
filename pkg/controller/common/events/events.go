// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package events

// Event reasons for the Elastic stack controller
const (
	// EventReasonDeprecated describes events that were due to a deprecated resource being submitted by the user.
	EventReasonDeprecated = "Deprecated"
	// EventReasonDelayed describes events where a requested change was delayed e.g. to prevent data loss.
	EventReasonDelayed = "Delayed"
	// EventReasonInvalidLicense describes events where a user configured an invalid license for the operator.
	EventReasonInvalidLicense = "InvalidLicense"
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

// Event actions for common controller actions
const (
	// EventActionValidation describes the validation step the controller was taking when the event was triggered
	EventActionValidation = "Validation"
	// EventActionAggregation describes the aggregation step the controller was taking when the event was triggered
	EventActionAggregation = "Aggregation"
	// EventActionCertificateReconciliation describes the certificate reconciliation step the controller was taking
	// when the event was triggered
	EventActionCertificateReconciliation = "CertificateReconciliation"
	// EventActionStatusUpdate describes the status update step the controller was taking when the event was triggered
	EventActionStatusUpdate = "StatusUpdate"
	// EventActionVersionCheck describes the version check step the controller was taking when the event was triggered
	EventActionVersionCheck = "VersionCheck"
	// EventActionAccessCheck describes the access check step the controller was taking when the event was triggered
	EventActionAccessCheck = "AccessCheck"
	// EventActionLicenseCheck describes the license check step the controller was taking when the event was triggered
	EventActionLicenseCheck = "LicenseCheck"
	// EventActionGetSecret describes the get secret step the controller was taking when the event was triggered
	EventActionGetSecret = "GetSecret"
	// EventActionParseSecret describes the parse secret step the controller was taking when the event was triggered
	EventActionParseSecret = "ParseSecret"
	// EventActionShutdown describes the shutdown step the controller was taking when the event was triggered
	EventActionShutdown = "Shutdown"
	// EventActionUpscale describes the upscale step the controller was taking when the event was triggered
	EventActionUpscale = "Upscale"
	// EventActionConfiguration describes the configuration step the controller was taking when the event was triggered
	EventActionConfiguration = "Configuration"
	// EventActionVersionUpgrade describes the version upgrade step the controller was taking when the event was triggered
	EventActionVersionUpgrade = "VersionUpgrade"
	// EventActionEnrollment describes the enrollment step the controller was taking when the event was triggered
	EventActionEnrollment = "Enrollment"
	// EventActionPolicyRetrieval describes the policy retrieval step the controller was taking when the event was triggered
	EventActionPolicyRetrieval = "PolicyRetrieval"
	// EventActionAssociationConfiguration describes the association configuration step the controller was taking when the event was triggered
	EventActionAssociationConfiguration = "AssociationConfiguration"
	// EventActionElasticsearchRetrieval describes the Elasticsearch retrieval step the controller was taking when the event was triggered
	EventActionElasticsearchRetrieval = "ElasticsearchRetrieval"
	// EventActionAnnotateResource describes the annotation step the controller was taking when the event was triggered
	EventActionAnnotateResource = "AnnotateResource"
	// EventActionDeploymentReconciliation describes the deployment reconciliation step the controller was taking when the event was triggered
	EventActionDeploymentReconciliation = "DeploymentReconciliation"
	// EventActionAssociationReconciliation describes the association reconciliation step the controller was taking when the event was triggered
	EventActionAssociationReconciliation = "AssociationReconciliation"
	// EventActionAutoOpsReconciliation = "AutoOpsReconciliation"
	EventActionAutoOpsReconciliation = "AutoOpsReconciliation"
	// EventActionRemoteClusterConfiguration describes the remote cluster configuration step the controller was taking when the event was triggered
	EventActionRemoteClusterConfiguration = "RemoteClusterConfiguration"
	// EventActionRemoteClusterAssociation describes the remote cluster association step the controller was taking when the event was triggered
	EventActionRemoteClusterAssociation = "RemoteClusterAssociation"
	// EventActionAssociationPreconditionCheck describes the association precondition check step the controller was taking when the event was triggered
	EventActionAssociationPreconditionCheck = "AssociationPreconditionCheck"
)

// Event is a k8s event that can be recorded via an event recorder.
type Event struct {
	EventType string
	Reason    string
	Action    string
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
func (r *Recorder) AddEvent(eventType, reason, action, message string) {
	if r.events == nil {
		r.events = []Event{}
	}
	r.events = append(r.events, Event{
		EventType: eventType,
		Reason:    reason,
		Action:    action,
		Message:   message,
	})
}

// Events returns all recorded events.
func (r *Recorder) Events() []Event {
	return r.events
}
