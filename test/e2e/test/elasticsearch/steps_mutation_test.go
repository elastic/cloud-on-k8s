// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_clusterUnavailability(t *testing.T) {
	// set an arbitrary large threshold we'll not reach
	cu := clusterUnavailability{threshold: 1 * time.Hour}

	// no threshold should be exceeded while the cluster is available
	require.False(t, cu.hasExceededThreshold())
	cu.markAvailable()
	require.True(t, cu.start.IsZero())
	require.False(t, cu.hasExceededThreshold())

	// mark the cluster as available, we're still below the threshold
	cu.markUnavailable()
	require.False(t, cu.start.IsZero())
	require.False(t, cu.hasExceededThreshold())

	// marking as unavailable again should not change the start time
	initialStartTime := cu.start
	cu.markUnavailable()
	require.Equal(t, initialStartTime, cu.start)
	require.False(t, cu.hasExceededThreshold())

	// marking as available again should reset the start time
	cu.markAvailable()
	require.True(t, cu.start.IsZero())
	require.False(t, cu.hasExceededThreshold())

	// simulate a lower threshold we should have exceeded
	cu.markUnavailable()
	cu.threshold = time.Duration(0)
	require.True(t, cu.hasExceededThreshold())
}
