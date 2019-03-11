// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_periodicExecForever(t *testing.T) {
	events := make(chan struct{}, 1)
	exec := func() (done bool, err error) {
		<-events
		return false, nil
	}
	pe := newPeriodicExec(exec, 1*time.Millisecond)
	// run until stopped
	done := make(chan error)
	go func() {
		done <- pe.Run()
	}()
	// should be executed at least 3 times
	events <- struct{}{}
	events <- struct{}{}
	events <- struct{}{}
	// close the channel so the exec func does nothing anymore
	close(events)
	// stop
	pe.Stop()
	// should be stopped, no error
	err := <-done
	require.NoError(t, err)
}

func Test_periodicExecReturnErr(t *testing.T) {
	exec := func() (done bool, err error) {
		return false, errors.New("err returned")
	}
	pe := newPeriodicExec(exec, 1*time.Millisecond)
	err := pe.Run()
	require.EqualError(t, err, "err returned")
}
func Test_periodicExecOnce(t *testing.T) {
	exec := func() (done bool, err error) {
		return true, nil
	}
	pe := newPeriodicExec(exec, 1*time.Millisecond)
	err := pe.Run()
	require.NoError(t, err)
}
