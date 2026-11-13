// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRBACOnRefsMode(t *testing.T) {
	for _, tt := range []struct {
		input   string
		want    RBACOnRefsMode
		wantErr bool
	}{
		{input: "false", want: RBACOnRefsModeOff},
		{input: "true", want: RBACOnRefsModeTrue},
		{input: "legacy", want: RBACOnRefsModeLegacy},
		{input: "all", want: RBACOnRefsModeAll},
		// backward-compatible bool aliases accepted by strconv.ParseBool
		{input: "1", want: RBACOnRefsModeTrue},
		{input: "t", want: RBACOnRefsModeTrue},
		{input: "T", want: RBACOnRefsModeTrue},
		{input: "TRUE", want: RBACOnRefsModeTrue},
		{input: "0", want: RBACOnRefsModeOff},
		{input: "f", want: RBACOnRefsModeOff},
		{input: "F", want: RBACOnRefsModeOff},
		{input: "FALSE", want: RBACOnRefsModeOff},
		{input: "", wantErr: true},
		{input: "yes", wantErr: true},
		{input: "ALL", wantErr: true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRBACOnRefsMode(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestRBACOnRefsMode_Methods(t *testing.T) {
	for _, tt := range []struct {
		mode                       RBACOnRefsMode
		wantEnforcementEnabled     bool
		wantEnforceAllAssociations bool
	}{
		{RBACOnRefsModeOff, false, false},
		{RBACOnRefsModeTrue, true, false},
		{RBACOnRefsModeLegacy, true, false},
		{RBACOnRefsModeAll, true, true},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			require.Equal(t, tt.wantEnforcementEnabled, tt.mode.EnforcementEnabled())
			require.Equal(t, tt.wantEnforceAllAssociations, tt.mode.EnforcementAllAssociations())
		})
	}
}

func TestRBACOnRefsMode_StartupMessage(t *testing.T) {
	for _, tt := range []struct {
		mode         RBACOnRefsMode
		wantContains []string
	}{
		{
			mode: RBACOnRefsModeOff,
		},
		{
			mode: RBACOnRefsModeTrue,
			wantContains: []string{
				"subject to change",
				"emit a warning",
				"--enforce-rbac-on-refs=all",
			},
		},
		{
			mode: RBACOnRefsModeLegacy,
			wantContains: []string{
				"deprecated",
				"will be removed",
				"identical to --enforce-rbac-on-refs=true",
			},
		},
		{
			mode: RBACOnRefsModeAll,
			wantContains: []string{
				"active for all cross-namespace associations",
				"will be blocked",
			},
		},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			message := tt.mode.StartupMessage()
			if len(tt.wantContains) == 0 {
				require.Empty(t, message)
				return
			}
			for _, expected := range tt.wantContains {
				require.Contains(t, message, expected)
			}
		})
	}
}
