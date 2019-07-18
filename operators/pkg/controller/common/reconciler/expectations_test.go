// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExpectations(t *testing.T) {
	expectations := NewExpectations()
	// check expectations that were not set
	obj := metav1.ObjectMeta{
		UID:        types.UID("abc"),
		Name:       "name",
		Namespace:  "namespace",
		Generation: 2,
	}
	require.True(t, expectations.GenerationExpected(obj))
	// set expectations
	expectations.ExpectGeneration(obj)
	// check expectations are met for this object
	require.True(t, expectations.GenerationExpected(obj))
	// but not for the same object with a smaller generation
	obj.Generation = 1
	require.False(t, expectations.GenerationExpected(obj))
	// a different object (different UID) should have expectations met
	obj.UID = types.UID("another")
	require.True(t, expectations.GenerationExpected(obj))
}
