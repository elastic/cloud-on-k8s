/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/version"
)

func TestMinVersionOrSkip(t *testing.T) {
	k8s := version.MustParseGeneric("v1.12.8-gke.10")
	test := version.MustParseGeneric("v1.12")
	require.True(t, k8s.AtLeast(test))
}
