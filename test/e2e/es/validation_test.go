// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build es || e2e

package es

import (
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
)

// TestElasticsearchCRDOpenAPIValidation tests that the CRD OpenAPI validation is correctly setup.
// It does not test every single validation, just checks validation exists.
func TestElasticsearchCRDOpenAPIValidation(t *testing.T) {
	// create an ES cluster with 0 NodeSet
	b := elasticsearch.NewBuilder("es-crd-validation")
	k := test.NewK8sClientOrFatal()
	// creation should be rejected
	rejected := false
	test.Eventually(func() error {
		err := k.CreateOrUpdate(&b.Elasticsearch)
		if err != nil && apierrors.IsInvalid(err) {
			// all good!
			rejected = true
			return nil
		}
		if err != nil && !apierrors.IsInvalid(err) {
			// unrelated error, retry
			return err
		}
		// we got no error, but creation should have been rejected
		rejected = false
		return nil
	})(t)
	require.True(t, rejected)
}
