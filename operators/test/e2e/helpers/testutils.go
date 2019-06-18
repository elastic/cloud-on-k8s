// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
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

// TestifyTestingTStub mocks testify's TestingT interface
// so that we can use assertions outside a testing context
type TestifyTestingTStub struct {
	err error
}

// Errorf sets the error for the TestingTStub
func (t *TestifyTestingTStub) Errorf(msg string, args ...interface{}) {
	t.err = fmt.Errorf(msg, args...)
}

// ElementsMatch checks that both given slices contain the same elements
func ElementsMatch(listA interface{}, listB interface{}) error {
	t := TestifyTestingTStub{}
	assert.ElementsMatch(&t, listA, listB)
	if t.err != nil {
		return t.err
	}
	return nil
}

// BoolPtr returns a pointer to a bool/
func BoolPtr(b bool) *bool {
	return &b
}
