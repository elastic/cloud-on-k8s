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
	t.Run("too long suffix should panic", func(t *testing.T) {
		require.Panics(t, func() {
			namer := Namer{MaxSuffixLength: 1}
			namer.Suffix("foo", "bar")
		})
	})

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
		{
			name: "too long owner name",
			namer: Namer{
				MaxSuffixLength: 20,
				DefaultSuffixes: []string{"default"},
			},
			args: args{
				ownerName: "this-owner-name-is-too-long-and-needs-to-be-trimmed-in-order-to-fit-the-suffix",
				suffixes:  []string{"bar", "baz"},
			},
			want: "this-owner-name-is-too-long-and-needs-to-be-tri-default-bar-baz",
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
