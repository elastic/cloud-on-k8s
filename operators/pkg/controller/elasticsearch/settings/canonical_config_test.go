// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalConfig_Render(t *testing.T) {
	config := MustCanonicalConfig(map[string]string{
		"aaa":        "aa  a",
		"bbb":        "b  bb",
		"aab":        "a a a",
		"withquotes": "aa\"bb\"aa",
		"zz":         "zzz  z z z",
	})
	output, err := config.Render()
	require.NoError(t, err)
	expected := []byte(`aaa: aa  a
aab: a a a
bbb: b  bb
withquotes: aa"bb"aa
zz: zzz  z z z
`)
	require.Equal(t, expected, output)
}

func TestCanonicalConfig_MergeWith(t *testing.T) {
	tests := []struct {
		name string
		c    *CanonicalConfig
		c2   *CanonicalConfig
		want *CanonicalConfig
	}{
		{
			name: "both empty",
			c:    NewCanonicalConfig(),
			c2:   NewCanonicalConfig(),
			want: NewCanonicalConfig(),
		},
		{
			name: "both nil",
			c:    nil,
			c2:   nil,
			want: nil,
		},
		{
			name: "c2 nil",
			c:    MustNewSingleValue("a", "b"),
			c2:   nil,
			want: MustNewSingleValue("a", "b"),
		},
		{
			name: "different values",
			c:    MustNewSingleValue("a", "b"),
			c2:   MustNewSingleValue("c", "d"),
			want: MustCanonicalConfig(map[string]string{"a": "b", "c": "d"}),
		},
		{
			name: "conflict: c2 has precedence",
			c:    MustNewSingleValue("a", "b"),
			c2:   MustCanonicalConfig(map[string]string{"c": "d", "a": "e"}),
			want: MustCanonicalConfig(map[string]string{"a": "e", "c": "d"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Merge mutates c
			require.NoError(t, tt.c.MergeWith(tt.c2))
			if diff := tt.c.Diff(tt.want, nil); diff != nil {
				t.Errorf("CanonicalConfig.MergeWith() = %v, want %v", diff, tt.want)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *CanonicalConfig
		wantErr bool
	}{
		{
			name:    "no input",
			input:   "",
			want:    NewCanonicalConfig(),
			wantErr: false,
		},
		{
			name:    "simple input",
			input:   "a: b\nc: d",
			want:    MustCanonicalConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "trim whitespaces",
			input:   "      a: b   \n      c: d     ",
			want:    MustCanonicalConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "tabs are invalid in YAML",
			input:   "\ta: b   \n    c:d     ",
			wantErr: true,
		},
		{
			name:    "trim whitespaces between key and value",
			input:   "a  :     b",
			want:    MustCanonicalConfig(map[string]string{"a": "b"}),
			wantErr: false,
		},
		{
			name:    "trim newlines",
			input:   "  \n    a: b   \n\n    c: d   \n\n ",
			want:    MustCanonicalConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "ignore comments",
			input:   "a: b\n#this is a comment\nc: d",
			want:    MustCanonicalConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "support quotes",
			input:   `a: "string in quotes"`,
			want:    MustCanonicalConfig(map[string]string{"a": `string in quotes`}),
			wantErr: false,
		},
		{
			name:    "support special characters",
			input:   `a: "${node.ip}%.:=+è! /"`,
			want:    MustCanonicalConfig(map[string]string{"a": `${node.ip}%.:=+è! /`}),
			wantErr: false,
		},
		{
			name:    "stop at first :",
			input:   "a: b: c: d: e",
			wantErr: true,
		},
		{
			name:    "invalid entry",
			input:   "not key value",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid entry among valid entries (is valid YAML)",
			input:   "a: b\n  not key value \n c:d",
			want:    MustCanonicalConfig(map[string]interface{}{"a": "b not key value c:d"}),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got == tt.want {
				return
			}

			if diff := got.Diff(tt.want, nil); diff != nil {
				gotRendered, err := got.Render()
				require.NoError(t, err)
				wantRendered, err := tt.want.Render()
				require.NoError(t, err)
				t.Errorf("ParseConfig(), want: %s, got: %s", wantRendered, gotRendered)
			}
		})
	}
}

func TestCanonicalConfig_Diff(t *testing.T) {
	type args struct {
		c2     *CanonicalConfig
		ignore []string
	}
	tests := []struct {
		name string
		c    *CanonicalConfig
		args args
		want []string
	}{
		{
			name: "nil diff",
			c:    nil,
			args: args{},
			want: nil,
		},
		{
			name: "lhs nil",
			c:    nil,
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": 1,
				}),
				ignore: nil,
			},
			want: []string{"a"},
		},
		{
			name: "rhs nil",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": 2,
			}),
			args: args{},
			want: []string{"a"},
		},
		{
			name: "flags up key difference",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": map[string]string{
					"b": "foo",
				},
			}),
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": map[string]string{
						"b": "foo",
						"c": "bar",
					},
				}),
			},
			want: []string{"a.c"},
		},
		{
			name: "flags up value difference",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": map[string]string{
					"b": "foo",
				},
			}),
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": map[string]int{
						"b": 1,
					},
				}),
			},
			want: []string{"a.b"},
		},
		{
			name: "respects ignore list",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": map[string]interface{}{
					"b": "foo",
					"c": []int{1, 2, 3},
				},
			}),
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": map[string]interface{}{
						"b": 1,
						"c": []int{1, 24},
					},
				}),
				ignore: []string{"a.b", "a.c"},
			},
			want: nil,
		},
		{
			name: "respects list order",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": map[string]interface{}{
					"b": []int{1, 2, 3},
				},
			}),
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": map[string]interface{}{
						"b": []int{1, 3, 2},
					},
				}),
			},
			want: []string{"a.b.1", "a.b.2"},
		},
		{
			name: "respects primitive types",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": 1,
				"b": 1.0,
				"c": "true",
			}),
			args: args{
				c2: MustCanonicalConfig(map[string]interface{}{
					"a": 1.0,
					"b": 1.0,
					"c": true,
				}),
			},
			want: []string{"a", "c"},
		},
		{
			name: "handles comparison of different types correctly",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": []string{"x", "y"},
			}),
			args: args{
				c2:     MustCanonicalConfig(map[string]interface{}{}),
				ignore: []string{"a"},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.Diff(tt.args.c2, tt.args.ignore); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CanonicalConfig.Diff() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testConfig struct {
	A []string `config:"a"`
}

func TestCanonicalConfig_SetStrings(t *testing.T) {
	type args struct {
		key  string
		vals []string
	}
	tests := []struct {
		name    string
		c       *CanonicalConfig
		args    args
		want    testConfig
		wantErr bool
	}{
		{
			name: "mutates config",
			c:    NewCanonicalConfig(),
			args: args{
				key:  "a",
				vals: []string{"foo", "bar"},
			},
			want:    testConfig{A: []string{"foo", "bar"}},
			wantErr: false,
		},
		{
			name: "always sets a list setting",
			c:    NewCanonicalConfig(),
			args: args{
				key:  "a",
				vals: []string{"foo"},
			},
			want:    testConfig{A: []string{"foo"}},
			wantErr: false,
		},
		{
			name:    "with nil argument",
			c:       NewCanonicalConfig(),
			args:    args{},
			wantErr: true,
		},
		{
			name: "with nil config",
			c:    nil,
			args: args{
				key:  "a",
				vals: []string{"a"},
			},
			wantErr: true,
		},
		{
			name: "already set",
			c: MustCanonicalConfig(map[string]interface{}{
				"a": []string{"foo", "bar", "baz"},
			}),
			args: args{
				key:  "a",
				vals: []string{"bizz", "buzz"},
			},
			want:    testConfig{A: []string{"bizz", "buzz", "baz"}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.c.SetStrings(tt.args.key, tt.args.vals...)
			if (err != nil) != tt.wantErr {
				t.Errorf("CanonicalConfig.SetStrings() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				var cmp testConfig
				require.NoError(t, tt.c.access().Unpack(&cmp))
				require.Equal(t, tt.want, cmp)
			}
		})
	}
}
