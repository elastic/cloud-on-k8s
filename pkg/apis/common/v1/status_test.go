// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestConditions_Index(t *testing.T) {
	tests := []struct {
		description string
		conditions  Conditions
		typ         ConditionType
		expected    int
	}{
		{
			description: "returns index of existing condition",
			conditions: Conditions{
				Condition{
					Type: OrchestrationPaused,
				},
				Condition{
					Type: ConditionType("foo"),
				},
			},
			typ:      OrchestrationPaused,
			expected: 0,
		},
		{
			description: "returns -1 if condition does not exist",
			conditions: Conditions{
				Condition{
					Type: OrchestrationPaused,
				},
				Condition{
					Type: ConditionType("foo"),
				},
			},
			typ:      ConditionType("bar"),
			expected: -1,
		},
		{
			description: "returns -1 if conditions is nil",
			conditions:  nil,
			typ:         ConditionType("bar"),
			expected:    -1,
		},
		{
			description: "returns -1 if conditions is empty",
			conditions:  Conditions{},
			typ:         ConditionType("bar"),
			expected:    -1,
		},
		{
			description: "returns -1 if type is empty",
			conditions: Conditions{
				Condition{
					Type: ConditionType("foo"),
				},
			},
			typ:      "",
			expected: -1,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			index := test.conditions.Index(test.typ)
			assert.Equalf(t, test.expected, index, "expected index to be %d, got %d", test.expected, index)
		})
	}
}

func TestConditions_MergeWith(t *testing.T) {
	tests := []struct {
		description   string
		conditions    Conditions
		newConditions Conditions
		expected      Conditions
	}{
		{
			description: "merges non-empty existing conditions with non-empty new conditions with no overlap",
			conditions: Conditions{
				Condition{
					Type: OrchestrationPaused,
				},
			},
			newConditions: Conditions{
				Condition{
					Type: "foo",
				},
			},
			expected: Conditions{
				Condition{
					Type: OrchestrationPaused,
				},
				Condition{
					Type: "foo",
				},
			},
		},
		{
			description: "merges non-empty existing conditions with non-empty new conditions where new conditions has status updates",
			conditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionFalse,
				},
			},
			newConditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
			expected: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
		},
		{
			description: "merges non-empty existing conditions with non-empty new conditions where new conditions has message updates",
			conditions: Conditions{
				Condition{
					Type:    OrchestrationPaused,
					Status:  v1.ConditionTrue,
					Message: "old message",
				},
			},
			newConditions: Conditions{
				Condition{
					Type:    OrchestrationPaused,
					Status:  v1.ConditionTrue,
					Message: "new message",
				},
			},
			expected: Conditions{
				Condition{
					Type:    OrchestrationPaused,
					Status:  v1.ConditionTrue,
					Message: "new message",
				},
			},
		},
		{
			description: "merges non-empty existing conditions with empty new conditions",
			conditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
			newConditions: Conditions{},
			expected: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
		},
		{
			description: "merges empty existing conditions with non-empty new conditions",
			conditions:  Conditions{},
			newConditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
			expected: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
		},
		{
			description:   "merges empty existing conditions with empty new conditions",
			conditions:    Conditions{},
			newConditions: Conditions{},
			expected:      Conditions{},
		},
		{
			description:   "merges nil existing conditions with nil new conditions",
			conditions:    nil,
			newConditions: nil,
			expected:      Conditions{},
		},
		{
			description: "merges nil existing conditions with non-empty new conditions",
			conditions:  nil,
			newConditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
			expected: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
		},
		{
			description: "merges non-empty existing conditions with nil new conditions",
			conditions: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
			newConditions: nil,
			expected: Conditions{
				Condition{
					Type:   OrchestrationPaused,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			output := test.conditions.MergeWith(test.newConditions...)
			assert.Equal(t, test.expected, output, "conditions not merged as expected")
		})
	}
}
