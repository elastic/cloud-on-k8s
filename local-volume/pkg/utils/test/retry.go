// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/local-volume/pkg/utils/retry"
	"github.com/stretchr/testify/assert"
)

// Default values to be used for testing purpose
const (
	Timeout       = time.Second * 5
	RetryInterval = time.Millisecond * 100
)

// RetryUntilSuccess calls retry.UntilSuccess with
// default timeout and retry interval,
// and asserts that no error is returned
func RetryUntilSuccess(t *testing.T, f func() error) {
	assert.NoError(t, retry.UntilSuccess(f, Timeout, RetryInterval))
}
