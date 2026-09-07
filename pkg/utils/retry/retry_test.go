// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

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

func TestRetryOnErrorRetriesMatchingError(t *testing.T) {
	retryableErr := errors.New("retryable")
	nAttempts := 0
	f := func() error {
		nAttempts++
		if nAttempts == 2 {
			return nil
		}
		return retryableErr
	}

	err := RetryOnError(
		f,
		func(err error) bool { return errors.Is(err, retryableErr) },
		10*time.Second,
		0,
	)

	assert.NoError(t, err)
	assert.Equal(t, 2, nAttempts)
}

func TestRetryOnErrorReturnsNonRetryableError(t *testing.T) {
	permanentErr := errors.New("permanent")
	nAttempts := 0
	f := func() error {
		nAttempts++
		return permanentErr
	}

	err := RetryOnError(f, func(error) bool { return false }, 10*time.Second, 0)

	assert.ErrorIs(t, err, permanentErr)
	assert.Equal(t, 1, nAttempts)
}

func TestRetryOnErrorReturnsNonRetryableErrorAfterRetry(t *testing.T) {
	retryableErr := errors.New("retryable")
	permanentErr := errors.New("permanent")
	nAttempts := 0
	f := func() error {
		nAttempts++
		if nAttempts == 1 {
			return retryableErr
		}
		return permanentErr
	}

	err := RetryOnError(
		f,
		func(err error) bool { return errors.Is(err, retryableErr) },
		10*time.Second,
		0,
	)

	assert.ErrorIs(t, err, permanentErr)
	assert.Equal(t, 2, nAttempts)
}

func TestRetryOnErrorWithNilPredicateRetriesAllErrors(t *testing.T) {
	nAttempts := 0
	f := func() error {
		nAttempts++
		if nAttempts == 2 {
			return nil
		}
		return errors.New("retryable")
	}

	assert.NoError(t, RetryOnError(f, nil, 10*time.Second, 0))
	assert.Equal(t, 2, nAttempts)
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
