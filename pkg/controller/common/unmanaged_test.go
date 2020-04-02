// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testcase struct {
	name string

	// annotationSequence is list of annotations that are simulated.
	annotationSequence []map[string]string

	// Expected (un)managed state.
	expectedState []bool
}

func TestUnmanagedCondition(t *testing.T) {
	var tests = []testcase{
		{
			name: "Simple unmanaged/managed simulation (a.k.a the Happy Path)",
			annotationSequence: []map[string]string{
				{ManagedAnnotation: "true"},
				{ManagedAnnotation: "false"},
				{ManagedAnnotation: "true"},
				{ManagedAnnotation: "false"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				true,
			},
		},
		{
			name: "Anything but 'false' means managed",
			annotationSequence: []map[string]string{
				{ManagedAnnotation: ""}, // empty annotation
				{ManagedAnnotation: "false"},
				{ManagedAnnotation: "XXXX"}, // unable to parse these
				{ManagedAnnotation: "1"},
				{ManagedAnnotation: "0"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				false,
				false,
			},
		},
		{
			name: "Still support legacy annotation",
			annotationSequence: []map[string]string{
				{LegacyPauseAnnoation: "true"}, // still support legacy for backwards compatibility
				{LegacyPauseAnnoation: "false"},
				{LegacyPauseAnnoation: "foo"},
				{LegacyPauseAnnoation: "false", ManagedAnnotation: "false"}, // new one takes precedence
				{LegacyPauseAnnoation: "true", ManagedAnnotation: "true"},   // but legacy is respected if true
			},
			expectedState: []bool{
				true,
				false,
				false,
				true,
				true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i, expectedState := range test.expectedState {
				meta := v1.ObjectMeta{
					Name:        "bar",
					Namespace:   "foo",
					Annotations: test.annotationSequence[i],
				}
				actualPauseState := IsUnmanaged(meta)
				assert.Equal(t, expectedState, actualPauseState, test.annotationSequence[i])
			}
		})
	}
}
