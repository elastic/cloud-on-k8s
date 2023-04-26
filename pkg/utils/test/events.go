package test

import (
	"k8s.io/client-go/tools/record"
	"testing"
	"time"
)

const (
	// EventsTimeout is na arbitrary timeout for events to be collected in a unit test.
	EventsTimeout = time.Second * 5
)

// ReadAtMostEvents attempts to read at most minEventCount from a FakeRecorder.
func ReadAtMostEvents(t *testing.T, minEventCount int, recorder *record.FakeRecorder) []string {
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