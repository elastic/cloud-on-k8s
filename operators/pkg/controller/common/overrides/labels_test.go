// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package overrides

import (
	"reflect"
	"testing"
)

func TestSetDefaultLabels(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string]string
		defaults map[string]string
		want     map[string]string
	}{
		{
			name:     "nil existing is correctly handled",
			existing: nil,
			defaults: map[string]string{"a": "b", "c": "d"},
			want:     map[string]string{"a": "b", "c": "d"},
		},
		{
			name:     "no conflict",
			existing: map[string]string{"a": "b", "c": "d"},
			defaults: map[string]string{"e": "f", "g": "h"},
			want:     map[string]string{"a": "b", "c": "d", "e": "f", "g": "h"},
		},
		{
			name:     "in case of conflict, keep existing value",
			existing: map[string]string{"a": "b", "c": "d"},
			defaults: map[string]string{"a": "conflicting", "e": "f"},
			want:     map[string]string{"a": "b", "c": "d", "e": "f"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetDefaultLabels(tt.existing, tt.defaults); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SetDefaultLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}
