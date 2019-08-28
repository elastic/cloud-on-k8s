// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNamer_WithDefaultSuffixes(t *testing.T) {
	type args struct {
		defaultSuffixes []string
	}
	tests := []struct {
		name  string
		namer Namer
		args  args
		want  func(t *testing.T, namer Namer)
	}{
		{
			name: "should replace suffixes",
			namer: Namer{
				MaxSuffixLength: 27,
				DefaultSuffixes: []string{"foo"},
			},
			args: args{
				defaultSuffixes: []string{"bar"},
			},
			want: func(t *testing.T, namer Namer) {
				require.Equal(t, "test-bar-123", namer.Suffix("test", "123"))
			},
		},
		{
			name: "should add suffixes when there is no suffix to begin with",
			namer: Namer{
				MaxSuffixLength: 27,
			},
			args: args{
				defaultSuffixes: []string{"foo"},
			},
			want: func(t *testing.T, namer Namer) {
				require.Equal(t, "test-foo-123", namer.Suffix("test", "123"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.namer.WithDefaultSuffixes(tt.args.defaultSuffixes...)
			tt.want(t, got)
		})
	}
}

func TestNamer_Suffix(t *testing.T) {
	type args struct {
		ownerName string
		suffixes  []string
	}
	tests := []struct {
		name  string
		namer Namer
		args  args
		want  string
	}{
		{
			name: "simple suffix",
			namer: Namer{
				MaxSuffixLength: 20,
			},
			args: args{ownerName: "foo", suffixes: []string{"bar"}},
			want: "foo-bar",
		},
		{
			name: "multiple suffixes",
			namer: Namer{
				MaxSuffixLength: 20,
			},
			args: args{ownerName: "foo", suffixes: []string{"bar", "baz"}},
			want: "foo-bar-baz",
		},
		{
			name: "default suffix",
			namer: Namer{
				MaxSuffixLength: 20,
				DefaultSuffixes: []string{"default"},
			},
			args: args{ownerName: "foo", suffixes: []string{"bar", "baz"}},
			want: "foo-default-bar-baz",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.namer.Suffix(tt.args.ownerName, tt.args.suffixes...); got != tt.want {
				t.Errorf("Namer.Suffix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamerSafeSuffixErrors(t *testing.T) {
	testCases := []struct {
		name      string
		namer     Namer
		ownerName string
		suffixes  []string
		wantName  string
	}{
		{
			name:      "long owner name",
			namer:     Namer{MaxSuffixLength: 20, DefaultSuffixes: []string{"es"}},
			ownerName: "extremely-long-and-unwieldy-name-for-owner-that-exceeds-the-limit",
			suffixes:  []string{"bar", "baz"},
			wantName:  "extremely-long-and-unwieldy-name-for-owner-that-exce-es-bar-baz",
		},
		{
			name:      "long suffixes",
			namer:     Namer{MaxSuffixLength: 20, DefaultSuffixes: []string{"es"}},
			ownerName: "test",
			suffixes:  []string{"bar", "baz", "very-long-suffix-exceeding-the-limit"},
			wantName:  "test-es-bar-baz-very-lon",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			haveName, err := tc.namer.SafeSuffix(tc.ownerName, tc.suffixes...)
			require.Error(t, err)
			require.Equal(t, tc.wantName, haveName)
		})
	}
}
