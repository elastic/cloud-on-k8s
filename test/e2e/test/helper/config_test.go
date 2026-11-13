// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package helper

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEffectiveConfig(t *testing.T) {
	// readLogs returns the content of a testdata file holding the first lines logged by an operator at startup,
	// extracted from the eck-diagnostics of an e2e run.
	readLogs := func(name string) []byte {
		logs, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		return logs
	}
	namespaceSelectorLogs := readLogs("operator-startup-namespace-selector.log")
	namespacesLogs := readLogs("operator-startup-namespaces.log")

	for _, tc := range []struct {
		name string
		logs []byte
		// wantValues holds the expected value of a subset of the configuration keys
		wantValues map[string]any
		// wantErr is a substring of the expected error, empty if no error is expected
		wantErr string
	}{
		{
			name: "operator started with a namespace selector",
			logs: namespaceSelectorLogs,
			wantValues: map[string]any{
				"namespace-selector": map[string]any{"matchlabels": map[string]any{"eck-visible": "true"}},
				"namespaces":         []any{},
				"operator-namespace": "e2e-66oml-elastic-system",
				"log-verbosity":      float64(1),
			},
		},
		{
			name: "operator started with a list of namespaces",
			logs: namespacesLogs,
			wantValues: map[string]any{
				"namespace-selector": nil,
				"namespaces":         []any{"e2e-66oml-mercury", "e2e-66oml-venus"},
				"operator-namespace": "e2e-66oml-elastic-system",
				"log-verbosity":      float64(1),
			},
		},
		{
			name: "logs truncated after the effective configuration line",
			logs: namespaceSelectorLogs[:len(namespaceSelectorLogs)-100],
			wantValues: map[string]any{
				"namespace-selector": map[string]any{"matchlabels": map[string]any{"eck-visible": "true"}},
				"namespaces":         []any{},
			},
		},
		{
			name:    "logs truncated within the effective configuration line",
			logs:    namespaceSelectorLogs[:500],
			wantErr: "log line not found",
		},
		{
			name:    "no effective configuration line",
			logs:    []byte("2026/10/05 07:00:34 INFO not a JSON line\n{\"message\":\"operator runs without FIPS mode\"}\n"),
			wantErr: "log line not found",
		},
		{
			name:    "empty logs",
			logs:    nil,
			wantErr: "log line not found",
		},
		{
			name:    "line exceeding the scanner buffer",
			logs:    []byte(strings.Repeat("x", effectiveConfigLogLimitBytes+1)),
			wantErr: "token too long",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values, err := parseEffectiveConfig(tc.logs)
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			require.Equal(t, tc.wantErr != "", err != nil, "unexpected error: %v", err)
			require.Contains(t, errMsg, tc.wantErr)
			for key, want := range tc.wantValues {
				require.Equal(t, want, values[key], "unexpected value for key %s", key)
			}
		})
	}
}

func TestNormalizeConfigValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		// expected is a value as read from the operator ConfigMap
		expected any
		// loaded is a value as read from the effective configuration log line
		loaded    any
		wantEqual bool
	}{
		{
			name:      "unset namespaces matches the logged empty list",
			expected:  nil,
			loaded:    []any{},
			wantEqual: true,
		},
		{
			name:      "unset value matches an empty string",
			expected:  nil,
			loaded:    "",
			wantEqual: true,
		},
		{
			name:      "unset value matches an empty map",
			expected:  nil,
			loaded:    map[string]any{},
			wantEqual: true,
		},
		{
			name:      "same namespaces",
			expected:  []any{"ns1", "ns2"},
			loaded:    []any{"ns1", "ns2"},
			wantEqual: true,
		},
		{
			name:      "same namespaces as string slice",
			expected:  []string{"ns1", "ns2"},
			loaded:    []any{"ns1", "ns2"},
			wantEqual: true,
		},
		{
			name:      "different namespaces",
			expected:  []any{"ns1"},
			loaded:    []any{"ns1", "ns2"},
			wantEqual: false,
		},
		{
			name:      "namespaces in a different order",
			expected:  []any{"ns1", "ns2"},
			loaded:    []any{"ns2", "ns1"},
			wantEqual: false,
		},
		{
			name:      "unset namespaces does not match a list of namespaces",
			expected:  nil,
			loaded:    []any{"ns1", "ns2"},
			wantEqual: false,
		},
		{
			name:      "namespace selector keys are case-insensitive",
			expected:  map[string]any{"matchLabels": map[string]any{"eck-visible": "true"}},
			loaded:    map[string]any{"matchlabels": map[string]any{"eck-visible": "true"}},
			wantEqual: true,
		},
		{
			name:      "namespace selector values are case-sensitive",
			expected:  map[string]any{"matchLabels": map[string]any{"eck-visible": "true"}},
			loaded:    map[string]any{"matchlabels": map[string]any{"eck-visible": "True"}},
			wantEqual: false,
		},
		{
			name:      "namespace selector does not match an unset selector",
			expected:  map[string]any{"matchLabels": map[string]any{"eck-visible": "true"}},
			loaded:    nil,
			wantEqual: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantEqual, reflect.DeepEqual(normalizeConfigValue(tc.expected), normalizeConfigValue(tc.loaded)))
		})
	}
}
