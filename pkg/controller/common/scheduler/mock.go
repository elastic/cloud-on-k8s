// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheduler

import (
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// MockScheduler implements the Scheduler interface for testing purposes.
// Events() will be nil and no timers will be created.
// To inspect scheduled events see Scheduled
type MockScheduler struct {
	Scheduled map[types.NamespacedName]time.Duration
}

func (m *MockScheduler) Events() chan event.GenericEvent {
	return nil
}

func (m *MockScheduler) Schedule(nsn types.NamespacedName, after time.Duration) {
	if m.Scheduled == nil {
		m.Scheduled = make(map[types.NamespacedName]time.Duration)
	}
	m.Scheduled[nsn] = after
}

var _ Scheduler = &MockScheduler{}
