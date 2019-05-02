// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package loader

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/test/e2e/helpers"
	"github.com/stretchr/testify/require"
)

// LoadDocumentsSteps bulk loads some documents and check that all of them have been created.
func LoadDocumentsSteps(loader *DocumentLoader, count int) []helpers.TestStep {
	return []helpers.TestStep{
		{
			Name: "Loading some documents should succeed",
			Test: func(t *testing.T) {
				err := loader.Load(count)
				require.NoError(t, err)
			},
		},
		{
			Name: "Document count should be ok",
			Test: func(t *testing.T) {
				loader.CheckDocuments(t)
			},
		},
	}
}

// CheckDocumentsStep returns a step that checks that documents are still in the cluster.
func CheckDocumentsStep(loader *DocumentLoader) helpers.TestStep {
	return helpers.TestStep{
		Name: "Document count should be ok",
		Test: func(t *testing.T) {
			loader.CheckDocuments(t)
		},
	}
}
