// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build es e2e

package es

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// TestElasticsearchCRDOpenAPIValidation tests that the CRD OpenAPI validation is correctly setup.
// It does not test every single validation, just checks validation exists.
func TestElasticsearchCRDOpenAPIValidation(t *testing.T) {
	// create an ES cluster with 0 NodeSet
	b := elasticsearch.NewBuilder("es-crd-validation")
	k := test.NewK8sClientOrFatal()
	// creation should be rejected
	err := k.Client.Create(context.Background(), &b.Elasticsearch)
	require.True(t, apierrors.IsInvalid(err))
}
