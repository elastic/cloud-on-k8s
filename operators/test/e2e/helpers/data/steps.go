// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package data

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

// LoadDataSteps bulk loads some data and check that all documents have been created.
func LoadDataSteps(loader *Loader, count int) []helpers.TestStep {
	return []helpers.TestStep{
		{
			Name: "Injecting data should succeed",
			Test: func(t *testing.T) {
				err := loader.Load(count)
				require.NoError(t, err)
			},
		},
		{
			Name: "Data count should be ok",
			Test: func(t *testing.T) {
				loader.CheckData(t)
			},
		},
	}
}

// CheckDataStep returns a step that checks that documents are still in the cluster.
func CheckDataStep(loader *Loader) helpers.TestStep {
	return helpers.TestStep{
		Name: "Data count should be ok",
		Test: func(t *testing.T) {
			loader.CheckData(t)
		},
	}
}
