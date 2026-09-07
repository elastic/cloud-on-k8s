// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package retry

import (
	"fmt"
	"time"
)

// ErrTimeoutReached is an error returned when timeout is reached
type ErrTimeoutReached struct {
	Timeout time.Duration
}

func (e *ErrTimeoutReached) Error() string {
	return fmt.Sprintf("timeout reached after %s", e.Timeout)
}

// UntilSuccess retries the given function f for up to the given timeout,
// by separating each attempt by the given retryInterval.
//
// f is considered successful if it does not return an error.
// In case the timeout is reached before the first failure of f,
// an ErrTimeoutReached is returned.
// Otherwise, the error from the last attempt is returned.
func UntilSuccess(f func() error, timeout time.Duration, retryInterval time.Duration) error {
	return RetryOnError(f, func(error) bool { return true }, timeout, retryInterval)
}

// RetryOnError retries the given function f for up to the given timeout,
// separating each attempt by the given retryInterval. An error is retried only
// when shouldRetry returns true. Non-retryable errors are returned immediately.
// A nil shouldRetry retries all errors.
//
// f is considered successful if it does not return an error. If the timeout is
// reached before the first failure of f, an ErrTimeoutReached is returned.
// Otherwise, the error from the last attempt is returned.
func RetryOnError(
	f func() error,
	shouldRetry func(error) bool,
	timeout time.Duration,
	retryInterval time.Duration,
) error {
	totalTimer := time.NewTimer(timeout)
	defer totalTimer.Stop()
	var lastErr error
	errorToReturn := func() error {
		if lastErr == nil {
			return &ErrTimeoutReached{Timeout: timeout}
		}
		return lastErr
	}
	for {
		resp := make(chan error, 1)
		go func() {
			resp <- f()
		}()
		select {
		case <-totalTimer.C:
			return errorToReturn()
		case err := <-resp:
			if err == nil {
				return nil
			}
			lastErr = err
			if shouldRetry != nil && !shouldRetry(err) {
				return err
			}
			retryTimer := time.NewTimer(retryInterval)
			select {
			case <-retryTimer.C:
				retryTimer.Stop()
				continue
			case <-totalTimer.C:
				retryTimer.Stop()
				return errorToReturn()
			}
		}
	}
}
