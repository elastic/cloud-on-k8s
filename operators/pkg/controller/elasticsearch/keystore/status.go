// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import "time"

// State of the Keystore updater
type State string

const (
	notInitializedState State = "notInitialized"
	waitingState        State = "waiting"
	runningState        State = "running"
	failedState         State = "failed"
)

// Status defined the observed state of a Keystore updater
type Status struct {
	State  State
	Reason string
	At     time.Time
}
