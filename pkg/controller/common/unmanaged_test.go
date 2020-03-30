// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"testing"

	log2 "github.com/elastic/cloud-on-k8s/pkg/utils/log"
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
	log2.InitLogger()
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
			name: "Can't parse or empty annotation",
			annotationSequence: []map[string]string{
				{ManagedAnnotation: ""}, // empty annotation
				{ManagedAnnotation: "false"},
				{ManagedAnnotation: "XXXX"}, // unable to parse this one
				{ManagedAnnotation: "1"},    // 1 == true
				{ManagedAnnotation: "0"},    // 0 == false
			},
			expectedState: []bool{
				false,
				true,
				false,
				false,
				true,
			},
		},
		{
			name: "Still support legacy annotation",
			annotationSequence: []map[string]string{
				{LegacyPauseAnnoation: "true"}, // still support legacy for backwards compatibility
				{LegacyPauseAnnoation: "false"},
				{LegacyPauseAnnoation: "foo"},
				{LegacyPauseAnnoation: "false", ManagedAnnotation: "false"}, // new one wins if both defined
				{LegacyPauseAnnoation: "true", ManagedAnnotation: "true"},
			},
			expectedState: []bool{
				true,
				false,
				false,
				true,
				false,
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
				assert.Equal(t, expectedState, actualPauseState)
			}
		})
	}
}
