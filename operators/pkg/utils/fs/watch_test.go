// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package fs

import (
	"errors"
	"io/ioutil"
	"os"
	"sync/atomic"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/utils/test"

	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func Test_WatchPath(t *testing.T) {
	logger := logf.Log.WithName("test")

	// setup a tmp file to watch
	tmpFile, err := ioutil.TempFile("", "tmpfile")
	require.NoError(t, err)
	path := tmpFile.Name()
	defer os.Remove(path)

	// count events of each category
	stopEvents := NewEventsCounter()
	stopEvents.Start()
	defer stopEvents.Stop()
	continueEvents := NewEventsCounter()
	continueEvents.Start()
	defer continueEvents.Stop()
	errorEvents := NewEventsCounter()
	errorEvents.Start()
	defer errorEvents.Stop()

	// on each event, read the file
	// according to its content, return in different ways
	f := func() (bool, error) {
		content, err := ioutil.ReadFile(path)
		require.NoError(t, err)
		switch string(content) {
		case "stop":
			stopEvents.Events <- "stop"
			return true, nil
		case "error":
			errorEvents.Events <- "error"
			return false, errors.New("error")
		default:
			continueEvents.Events <- "continue"
			return false, nil
		}
	}

	done := make(chan error)
	go func() {
		done <- WatchPath(path, f, logger)
	}()

	// should trigger an event before actually watching
	assertEventObserved(t, continueEvents)

	// trigger a change that should continue
	continueEvents.SetLastObserved()
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	assertEventObserved(t, continueEvents)
	// again
	continueEvents.SetLastObserved()
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	assertEventObserved(t, continueEvents)

	// trigger a change that should stop
	stopEvents.SetLastObserved()
	require.NoError(t, ioutil.WriteFile(path, []byte("stop"), 644))
	assertEventObserved(t, stopEvents)
	// WatchPath should return
	require.NoError(t, <-done)

	// run it again
	require.NoError(t, ioutil.WriteFile(path, []byte("continue"), 644))
	done = make(chan error)
	go func() {
		done <- WatchPath(path, f, logger)
	}()

	// should trigger an event before actually watching
	assertEventObserved(t, continueEvents)

	// trigger a change that should return an error
	errorEvents.SetLastObserved()
	require.NoError(t, ioutil.WriteFile(path, []byte("error"), 644))
	assertEventObserved(t, errorEvents)

	// WatchPath should return with an error
	require.Error(t, <-done, "error")
}

// assertEventObserved verifies that an event is eventually observed
// by the provided eventsCounter
func assertEventObserved(t *testing.T, eventsCounter *EventsCounter) {
	test.RetryUntilSuccess(t, func() error {
		if !eventsCounter.CountIncreased() {
			return errors.New("No event observed")
		}
		return nil
	})
}

// EventsCounter counts events provided to the Events channel
type EventsCounter struct {
	Events            chan string
	stop              chan struct{}
	count             int32
	lastObservedCount int32
}

// NewEventsCounter returns an initialized EventsCounter
func NewEventsCounter() *EventsCounter {
	var count, lastObservedCount int32
	atomic.StoreInt32(&count, 0)
	atomic.StoreInt32(&lastObservedCount, 0)
	return &EventsCounter{
		Events:            make(chan string),
		stop:              make(chan struct{}),
		count:             count,
		lastObservedCount: lastObservedCount,
	}
}

// Start counts events until stopped
func (e *EventsCounter) Start() {
	go func() {
		for {
			select {
			case <-e.stop:
				return
			case <-e.Events:
				atomic.AddInt32(&e.count, 1)
			}
		}
	}()
}

// Stop counting
func (e *EventsCounter) Stop() {
	close(e.stop)
}

// SetLastObserved keep tracks of the current count
func (e *EventsCounter) SetLastObserved() {
	atomic.StoreInt32(&e.lastObservedCount, atomic.LoadInt32(&e.count))
}

// CountIncreased returns true if the current count is higher
// than the last observed one
func (e *EventsCounter) CountIncreased() bool {
	count := atomic.LoadInt32(&e.count)
	lastObserved := atomic.LoadInt32(&e.lastObservedCount)
	return count > lastObserved
}
