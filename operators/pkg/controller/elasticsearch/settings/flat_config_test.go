// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlatConfig_Render(t *testing.T) {
	config := MustFlatConfig(map[string]string{
		"aaa":        "aa  a",
		"bbb":        "b  bb",
		"aab":        "a a a",
		"withquotes": "aa\"bb\"aa",
		"zz":         "zzz  z z z",
	})
	output, err := config.Render()
	require.NoError(t, err)
	expected := []byte(`# --- auto-generated ---
aaa: aa  a
aab: a a a
bbb: b  bb
withquotes: aa"bb"aa
zz: zzz  z z z
# --- end auto-generated ---
`)
	require.Equal(t, expected, output)
}

func TestFlatConfig_MergeWith(t *testing.T) {
	tests := []struct {
		name string
		c    *FlatConfig
		c2   *FlatConfig
		want *FlatConfig
	}{
		{
			name: "both empty",
			c:    NewFlatConfig(),
			c2:   NewFlatConfig(),
			want: NewFlatConfig(),
		},
		{
			name: "both nil",
			c:    nil,
			c2:   nil,
			want: NewFlatConfig(),
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
			want: MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
		},
		{
			name: "conflict: c2 has precedence",
			c:    MustNewSingleValue("a", "b"),
			c2:   MustFlatConfig(map[string]string{"c": "d", "a": "e"}),
			want: MustFlatConfig(map[string]string{"a": "e", "c": "d"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// c and c2 should remain unmodified
			if got := tt.c.MergeWith(tt.c2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FlatConfig.MergeWith() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *FlatConfig
		wantErr bool
	}{
		{
			name:    "no input",
			input:   "",
			want:    &FlatConfig{},
			wantErr: false,
		},
		{
			name:    "simple input",
			input:   "a: b\nc:d",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "trim whitespaces",
			input:   "      a: b   \n    c:d     ",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "trim tabs",
			input:   "\ta: b   \n    c:d     ",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "trim whitespaces between key and value",
			input:   "a  :     b",
			want:    MustFlatConfig(map[string]string{"a": "b"}),
			wantErr: false,
		},
		{
			name:    "trim newlines",
			input:   "  \n    a: b   \n\n    c:d    \n\n ",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "ignore comments",
			input:   "a: b\n #this is a comment\n c: d",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "support quotes",
			input:   `a: "string in quotes"`,
			want:    MustFlatConfig(map[string]string{"a": `"string in quotes"`}),
			wantErr: false,
		},
		{
			name:    "support special characters",
			input:   `a: %.:=+è! /\$`,
			want:    MustFlatConfig(map[string]string{"a": `%.:=+è! /\$`}),
			wantErr: false,
		},
		{
			name:    "stop at first :",
			input:   "a: b: c: d: e",
			want:    MustFlatConfig(map[string]string{"a": "b: c: d: e"}),
			wantErr: false,
		},
		{
			name:    "invalid entry",
			input:   "not key value",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid entry among valid entries",
			input:   "a: b\n  not key value \n c:d",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
