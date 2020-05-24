// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Reconcile(t *testing.T) {
	params := DriverParams{
		DaemonSet:  &DaemonSetSpec{},
		Deployment: &DeploymentSpec{},
	}

	result := Reconcile(params, nil, "", nil)
	require.True(t, result.Results.HasError())

	result = Reconcile(DriverParams{}, nil, "", nil)
	require.True(t, result.Results.HasError())
}
