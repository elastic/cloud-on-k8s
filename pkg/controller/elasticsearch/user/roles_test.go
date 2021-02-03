// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"reflect"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
)

func Test_RolesFileContent(t *testing.T) {
	rolesBytes := []byte(`another_role:
  cluster: [ 'all' ]
click_admins:
  run_as: [ 'clicks_watcher_1' ]
  cluster: [ 'monitor' ]
  indices:
  - names: [ 'events-*' ]
    privileges: [ 'read' ]
    field_security:
      grant: ['category', '@timestamp', 'message' ]
    query: '{"match": {"category": "click"}}'`)

	// unmarshal into rolesFileContent
	var r RolesFileContent
	require.NoError(t, yaml.Unmarshal(rolesBytes, &r))
	// should be a map[string]interface{} whose keys are the role names
	require.Len(t, r, 2)
	require.NotEmpty(t, r["click_admins"])
	require.NotEmpty(t, r["another_role"])
	require.Empty(t, r["unknown_role"])
	// .fileBytes() should unmarshal back to the original yaml but may be formatted differently (indent, lists, etc.)
	// so we don't compare the raw yaml explicitly here
	asBytes, err := r.FileBytes()
	require.NoError(t, err)
	// instead, compare the internal representation
	// unmarshal yaml back into rolesFileContent again
	var rAgain RolesFileContent
	require.NoError(t, yaml.Unmarshal(asBytes, &rAgain))
	// the initial yaml and the serialized/de-serialized rolesFileContent should be the same
	require.Equal(t, r, rAgain)
}

func TestRolesFileContent_MergeWith(t *testing.T) {
	type args struct {
		other RolesFileContent
	}
	tests := []struct {
		name   string
		r      RolesFileContent
		args   args
		want   RolesFileContent
		assert func(r, other, result RolesFileContent)
	}{

		{
			name: "when r is nil",
			r:    nil,
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
		},
		{
			name: "when other is nil",
			r:    RolesFileContent(map[string]interface{}{"a": "c"}),
			args: args{
				other: nil,
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
		},
		{
			name: "when both are nil",
			r:    nil,
			args: args{
				other: nil,
			},
			want: RolesFileContent(map[string]interface{}{}),
		},
		{
			name: "when r is empty",
			r:    RolesFileContent(map[string]interface{}{}),
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
		},
		{
			name: "when other is empty",
			r:    RolesFileContent(map[string]interface{}{"a": "c"}),
			args: args{
				other: RolesFileContent(map[string]interface{}{}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
		},
		{
			name: "when r has more items",
			r:    RolesFileContent(map[string]interface{}{"a": "b", "d": "e"}),
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c", "d": "e"}),
		},
		{
			name: "when other has more items",
			r:    RolesFileContent(map[string]interface{}{"a": "b"}),
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c", "d": "e"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c", "d": "e"}),
		},
		{
			name: "does give priority to other",
			r:    RolesFileContent(map[string]interface{}{"a": "b"}),
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
		},
		{
			name: "does not mutate in place",
			r:    RolesFileContent(map[string]interface{}{"a": "b"}),
			args: args{
				other: RolesFileContent(map[string]interface{}{"a": "c"}),
			},
			want: RolesFileContent(map[string]interface{}{"a": "c"}),
			assert: func(r, other, result RolesFileContent) {
				require.Equal(t, r, RolesFileContent(map[string]interface{}{"a": "b"}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.MergeWith(tt.args.other)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeWith() = %v, want %v", got, tt.want)
			}
			if tt.assert != nil {
				tt.assert(tt.r, tt.args.other, got)
			}
		})
	}
}
