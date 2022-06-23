// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/retry"
)

// Default values to be used for testing purpose.
const (
	Timeout       = time.Second * 10
	RetryInterval = time.Millisecond * 100
)

// RetryUntilSuccess calls retry.UntilSuccess with default timeout and retry interval,
// and requires that no error is returned.
func RetryUntilSuccess(t *testing.T, f func() error) {
	t.Helper()
	require.NoError(t, retry.UntilSuccess(f, Timeout, RetryInterval))
}
