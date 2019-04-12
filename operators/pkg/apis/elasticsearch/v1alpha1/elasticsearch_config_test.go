// // Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// // or more contributor license agreements. Licensed under the Elastic License;
// // you may not use this file except in compliance with the Elastic License.
//
package v1alpha1

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
)

func TestConfig_EqualRoles(t *testing.T) {
	type args struct {
		c2 Config
	}
	tests := []struct {
		name string
		c    Config
		args args
		want bool
	}{
		{
			name: "empty is equal",
			c:    Config{},
			args: args{},
			want: true,
		},
		{
			name: "same is equal",
			c: Config{
				Data: map[string]interface{}{
					NodeMaster: "true",
				},
			},
			args: args{
				c2: Config{
					Data: map[string]interface{}{
						NodeMaster: "true",
					},
				},
			},
			want: true,
		},
		{
			name: "detect differences",
			c: Config{
				Data: map[string]interface{}{
					NodeMaster: "false",
					NodeData:   "true",
				},
			},
			args: args{
				c2: Config{
					Data: map[string]interface{}{
						NodeData: "true",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c1, err := tt.c.Unpacked()
			require.NoError(t, err)
			c2, err := tt.args.c2.Unpacked()
			require.NoError(t, err)
			if got := c1.EqualRoles(c2); got != tt.want {
				t.Errorf("Config.EqualRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}

var testFixture = Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": 1,
		},
		"a.b.foo": "bar",
		"e":       []interface{}{1, 2, 3},
		"f":       true,
	},
}

var expectedJSONized = Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": 1.0,
			},
			"d": float64(1),
		},
		"a.b.foo": "bar",
		"e":       []interface{}{float64(1), float64(2), float64(3)},
		"f":       true,
	},
}

var expectedUnpacked = Config{
	Data: map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c":   1.0,
				"foo": "bar",
			},
			"d": uint64(1),
		},
		"e": []interface{}{uint64(1), uint64(2), uint64(3)},
		"f": true,
	},
}

func TestConfig_DeepCopyInto(t *testing.T) {
	tests := []struct {
		name     string
		in       Config
		expected Config
	}{
		{
			name:     "simple copy",
			in:       testFixture,
			expected: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out Config
			tt.in.DeepCopyInto(&out)
			if diff := deep.Equal(out, tt.expected); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_DeepCopy(t *testing.T) {
	tests := []struct {
		name string
		in   Config
		want Config
	}{
		{
			name: "simple copy",
			in:   testFixture,
			want: expectedJSONized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := deep.Equal(tt.in.DeepCopy(), &tt.want); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestConfig_Canonicalize(t *testing.T) {
	tests := []struct {
		name    string
		args    Config
		wantErr bool
	}{
		{
			name:    "simple test",
			args:    testFixture,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.args.Canonicalize()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Canonicalize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var out map[string]interface{}
			err = got.Unpack(&out)
			if (err != nil) != tt.wantErr {
				t.Errorf("ucfg.Config.Unpack error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := deep.Equal(expectedUnpacked.Data, out); diff != nil {
				t.Error(diff)
			}
		})
	}
}
