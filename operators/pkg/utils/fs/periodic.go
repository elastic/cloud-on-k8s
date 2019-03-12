// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fs

import (
	"context"
	"time"
)

// execFunction is a function to be executed periodically.
// If it returns an error, CallPeriodically will stop watching and return the error.
// If it returns true, CallPeriodically will stop watching with no error.
type execFunction func() (done bool, err error)

// CallPeriodically periodically executes the given function.
// It returns on:
// - context cancelled
// - f returning true
// - f returning an error
// Otherwise, it runs forever.
func CallPeriodically(ctx context.Context, f execFunction, period time.Duration) error {
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			done, err := f()
			if err != nil || done {
				return err
			}
		}
	}
}
