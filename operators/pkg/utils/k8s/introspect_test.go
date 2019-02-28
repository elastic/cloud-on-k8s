// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentNamespace(t *testing.T) {
	// from env
	os.Setenv("POD_NAMESPACE", "my-namespace")
	ns, err := CurrentNamespace()
	require.NoError(t, err)
	require.Equal(t, "my-namespace", ns)

	// from disk: not easy to test in heteregeneous environments :(
}

func TestCurrentPodName(t *testing.T) {
	// from env
	os.Setenv("POD_NAME", "my-pod")
	ns, err := CurrentPodName()
	require.NoError(t, err)
	require.Equal(t, "my-pod", ns)

	// from disk: not easy to test in heteregeneous environments :(
}
