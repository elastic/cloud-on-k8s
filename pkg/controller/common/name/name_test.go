// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package name

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/validation"
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
				MaxNameLength:   36,
				DefaultSuffixes: []string{"foo"},
			},
			args: args{
				defaultSuffixes: []string{"bar"},
			},
			want: func(t *testing.T, namer Namer) {
				t.Helper()
				require.Equal(t, "test-bar-123", namer.Suffix("test", "123"))
			},
		},
		{
			name: "should add suffixes when there is no suffix to begin with",
			namer: Namer{
				MaxNameLength:   36,
				MaxSuffixLength: 27,
			},
			args: args{
				defaultSuffixes: []string{"foo"},
			},
			want: func(t *testing.T, namer Namer) {
				t.Helper()
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
				MaxNameLength:   36,
				MaxSuffixLength: 20,
			},
			args: args{ownerName: "foo", suffixes: []string{"bar"}},
			want: "foo-bar",
		},
		{
			name: "multiple suffixes",
			namer: Namer{
				MaxNameLength:   36,
				MaxSuffixLength: 20,
			},
			args: args{ownerName: "foo", suffixes: []string{"bar", "baz"}},
			want: "foo-bar-baz",
		},
		{
			name: "default suffix",
			namer: Namer{
				MaxNameLength:   36,
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
			namer:     Namer{MaxSuffixLength: 20, MaxNameLength: 36, DefaultSuffixes: []string{"es"}},
			ownerName: "extremely-long-and-unwieldy-name-for-owner-that-exceeds-the-limit",
			suffixes:  []string{"bar", "baz"},
			wantName:  "extremely-long-and-unwiel-es-bar-baz",
		},
		{
			name:      "long suffixes",
			namer:     Namer{MaxSuffixLength: 20, MaxNameLength: 36, DefaultSuffixes: []string{"es"}},
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

func TestDNSLabel(t *testing.T) {
	exactly63 := strings.Repeat("a", validation.DNS1123LabelMaxLength)
	exactly64 := strings.Repeat("a", validation.DNS1123LabelMaxLength+1)
	// Truncation lands on a hyphen so the helper must strip it to stay a DNS label.
	hyphenAtBoundary := strings.Repeat("a", 53) + "-" + strings.Repeat("b", 20)
	customerCA := "es-ca-elasticsearch-aws-nonprod-monitoring-apl-ops-intelligence-monitoring"
	otherCA := "es-ca-elasticsearch-aws-nonprod-monitoring-apl-ops-intelligence-monitorinx"

	tests := []struct {
		name      string
		input     string
		wantSame  bool
		wantTrunc bool
	}{
		{name: "short name is unchanged", input: "es-ca-es-cluster-default", wantSame: true},
		{name: "63 character name is unchanged", input: exactly63, wantSame: true},
		{name: "64 character name is truncated with hash", input: exactly64, wantTrunc: true},
		{name: "customer reproduction is truncated with hash", input: customerCA, wantTrunc: true},
		{name: "truncation point on hyphen still yields a valid label", input: hyphenAtBoundary, wantTrunc: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DNSLabel(tt.input)
			require.LessOrEqual(t, len(got), validation.DNS1123LabelMaxLength)
			require.Empty(t, validation.IsDNS1123Label(got), got)
			require.Equal(t, got, DNSLabel(tt.input), "must be deterministic")
			if tt.wantSame {
				require.Equal(t, tt.input, got)
			}
			if tt.wantTrunc {
				require.NotEqual(t, tt.input, got)
				require.Regexp(t, `-[0-9a-f]{8}$`, got)
			}
		})
	}

	t.Run("nearby long names do not collide", func(t *testing.T) {
		require.NotEqual(t, DNSLabel(customerCA), DNSLabel(otherCA))
	})
}
