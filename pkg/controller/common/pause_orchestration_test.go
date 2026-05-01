// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsOrchestrationPaused(t *testing.T) {
	tests := []testcase{
		{
			name: "Simple paused-orchestration simulation (a.k.a the Happy Path)",
			annotationSequence: []map[string]string{
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "false"},
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "false"},
			},
			expectedState: []bool{
				true,
				false,
				true,
				false,
			},
		},
		{
			name: "Only 'true' means paused",
			annotationSequence: []map[string]string{
				{PauseOrchestrationAnnotation: ""}, // empty annotation
				{PauseOrchestrationAnnotation: "true"},
				{PauseOrchestrationAnnotation: "XXXX"}, // unable to parse these
				{PauseOrchestrationAnnotation: "1"},
				{PauseOrchestrationAnnotation: "0"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				false,
				false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i, expectedState := range test.expectedState {
				// testing with a secret, but could be any kind
				obj := corev1.Secret{ObjectMeta: v1.ObjectMeta{
					Name:        "bar",
					Namespace:   "foo",
					Annotations: test.annotationSequence[i],
				}}
				actualPauseState := IsOrchestrationPaused(&obj)
				assert.Equal(t, expectedState, actualPauseState, test.annotationSequence[i])
			}
		})
	}
}
