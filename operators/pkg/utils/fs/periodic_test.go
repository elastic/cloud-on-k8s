// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_CallPeriodically(t *testing.T) {
	events := make(chan struct{}, 1)
	exec := func() (done bool, err error) {
		<-events
		return false, nil
	}

	// run until stopped
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- CallPeriodically(ctx, exec, 1*time.Millisecond)
	}()

	// should be executed at least 3 times
	events <- struct{}{}
	events <- struct{}{}
	events <- struct{}{}

	// cancel the context
	cancel()
	err := <-done
	require.NoError(t, err)
}

func Test_CallPeriodicallyReturnErr(t *testing.T) {
	exec := func() (done bool, err error) {
		return false, errors.New("err returned")
	}
	err := CallPeriodically(context.Background(), exec, 1*time.Millisecond)
	require.EqualError(t, err, "err returned")
}

func Test_CallPeriodicallyExecOnce(t *testing.T) {
	exec := func() (done bool, err error) {
		return true, nil
	}
	err := CallPeriodically(context.Background(), exec, 1*time.Millisecond)
	require.NoError(t, err)
}
