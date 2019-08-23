// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package retry

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFirstTimeSuccess(t *testing.T) {
	f := func() error {
		return nil
	}
	assert.NoError(t, UntilSuccess(f, 10*time.Second, 0*time.Second))
}

func TestLaterSuccess(t *testing.T) {
	nAttempts := 0
	succeedAtAttempt := 2
	f := func() error {
		nAttempts++
		if nAttempts == succeedAtAttempt {
			return nil
		}
		return errors.New("not yet")
	}
	assert.NoError(t, UntilSuccess(f, 10*time.Second, 0*time.Second))
}

func TestGlobalTimeoutOnFirstCall(t *testing.T) {
	timeout := 1 * time.Millisecond
	stopChan := make(chan (struct{}))
	f := func() error {
		<-stopChan
		return nil
	}
	assert.EqualError(t, UntilSuccess(f, timeout, 0*time.Second), "timeout reached after 1ms")
	close(stopChan)
}

func TestGlobalTimeoutAfterFailures(t *testing.T) {
	f := func() error {
		return errors.New("i keep on failing")
	}
	assert.EqualError(t, UntilSuccess(f, 10*time.Millisecond, 0*time.Second), "i keep on failing")
}
