// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
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
	expected := []byte(`aaa: aa  a
aab: a a a
bbb: b  bb
withquotes: aa"bb"aa
zz: zzz  z z z
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
			// Merge mutates c
			require.NoError(t, tt.c.MergeWith(tt.c2))
			if diff := tt.c.Diff(tt.want, nil); diff != nil {
				t.Errorf("FlatConfig.MergeWith() = %v, want %v", diff, tt.want)
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
			want:    NewFlatConfig(),
			wantErr: false,
		},
		{
			name:    "simple input",
			input:   "a: b\nc: d",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "trim whitespaces",
			input:   "      a: b   \n      c: d     ",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
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
			want:    MustFlatConfig(map[string]string{"a": "b"}),
			wantErr: false,
		},
		{
			name:    "trim newlines",
			input:   "  \n    a: b   \n\n    c: d   \n\n ",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "ignore comments",
			input:   "a: b\n#this is a comment\nc: d",
			want:    MustFlatConfig(map[string]string{"a": "b", "c": "d"}),
			wantErr: false,
		},
		{
			name:    "support quotes",
			input:   `a: "string in quotes"`,
			want:    MustFlatConfig(map[string]string{"a": `string in quotes`}),
			wantErr: false,
		},
		{
			name:    "support special characters",
			input:   `a: "${node.ip}%.:=+è! /"`,
			want:    MustFlatConfig(map[string]string{"a": `${node.ip}%.:=+è! /`}),
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
			want:    MustFlatConfig(map[string]interface{}{"a": "b not key value c:d"}),
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
