// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"time"
)

// State of the Keystore updater
type State string

const (
	notInitializedState State = "notInitialized"
	WaitingState        State = "waiting"
	RunningState        State = "running"
	FailedState         State = "failed"

	KeystoreUpdatedReason        = "Keystore updated"
	secureSettingsReloadedReason = "Secure settings reloaded"
)

// Status defined the observed state of a Keystore updater
type Status struct {
	State  State
	Reason string
	At     time.Time
}

func (s State) ToString() string {
	return string(s)
}
