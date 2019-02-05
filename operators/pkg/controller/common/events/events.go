package events

// Event reasons for the Elastic stack controller
const (
	// EventReasonCreated describes events where resources were created.
	EventReasonCreated = "Created"
	// EventReasonDeleted describes events where resources were deleted.
	EventReasonDeleted = "Deleted"
	// EventReasonDelayed describes events where a requested change was delayed e.g. to prevent data loss.
	EventReasonDelayed = "Delayed"
	// EventReasonUnhealthy describes events where a stack deployments health was affected negatively.
	EventReasonUnhealthy = "Unhealthy"
	// EventReasonUnexpected describes events that were not anticipated or happened at an unexpected time.
	EventReasonUnexpected = "Unexpected"
	// EventReasonStateChange describes events that are expected state changes in a Elasticsearch cluster.
	EventReasonStateChange = "StateChange"
)
