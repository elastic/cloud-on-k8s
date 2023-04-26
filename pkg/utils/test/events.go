// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"
	"time"

	"k8s.io/client-go/tools/record"
)

const (
	// EventsTimeout is na arbitrary timeout for events to be collected in a unit test.
	EventsTimeout = time.Second * 5
)

// ReadAtMostEvents attempts to read at most minEventCount from a FakeRecorder.
func ReadAtMostEvents(t *testing.T, minEventCount int, recorder *record.FakeRecorder) []string {
	t.Helper()
	if minEventCount == 0 {
		return nil
	}
	c := time.After(EventsTimeout)
	gotEvents := make([]string, 0, minEventCount)
	gotEventCount := 0
	for {
		select {
		case e := <-recorder.Events:
			gotEvents = append(gotEvents, e)
			gotEventCount++
			if gotEventCount == minEventCount {
				// We have read the min. expected number of events
				return gotEvents
			}
		case <-c:
			t.Errorf("Timeout: expected at least %d events, got %d", minEventCount, gotEventCount)
			return gotEvents
		}
	}
}
