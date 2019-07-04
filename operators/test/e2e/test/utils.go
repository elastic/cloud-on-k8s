// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/retry"
	"github.com/stretchr/testify/require"
)

const (
	DefaultRetryDelay = 3 * time.Second
	defaultTimeout    = 5 * time.Minute
)

// ExitOnErr exits with code 1 if the given error is not nil
func ExitOnErr(err error) {
	if err != nil {
		fmt.Println(err)
		fmt.Println("Exiting.")
		os.Exit(1)
	}
}

// Eventually runs the given function until success,
// with a default timeout
func Eventually(f func() error) func(*testing.T) {
	return func(t *testing.T) {
		fmt.Printf("Retries (%s timeout): ", defaultTimeout)
		err := retry.UntilSuccess(func() error {
			fmt.Print(".") // super modern progress bar 2.0!
			return f()
		}, defaultTimeout, DefaultRetryDelay)
		fmt.Println()
		require.NoError(t, err)
	}
}

// BoolPtr returns a pointer to a bool/
func BoolPtr(b bool) *bool {
	return &b
}
