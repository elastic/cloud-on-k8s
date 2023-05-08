// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"

	"k8s.io/client-go/tools/record"
)

// ReadAtMostEvents attempts to read at most minEventCount from a FakeRecorder.
// This functions assumes that all the events are available in recorder.
func ReadAtMostEvents(t *testing.T, minEventCount int, recorder *record.FakeRecorder) []string {
	t.Helper()
	if minEventCount == 0 {
		return nil
	}
	gotEvents := make([]string, 0, minEventCount)
	gotEventCount := 0
	for i := 0; i < minEventCount; i++ {
		select {
		case e := <-recorder.Events:
			gotEvents = append(gotEvents, e)
			gotEventCount++
		default:
			t.Errorf("expected at least %d events, got %d", minEventCount, gotEventCount)
		}
	}
	return gotEvents
}
