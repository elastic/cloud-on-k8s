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

func MinVersionOrSkip(t *testing.T, v string) {
	info, err := ServerVersion()
	require.NoError(t, err)

	min := version.MustParseGeneric(v)
	actual := version.MustParseGeneric(info.GitVersion)
	if !actual.AtLeast(min) {
		t.SkipNow()
	}
}
