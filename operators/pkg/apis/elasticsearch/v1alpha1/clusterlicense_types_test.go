// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLicenseTypeFromString(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    LicenseType
		wantErr string
	}{
		{
			name:    "invalid type",
			args:    "enterprise",
			want:    LicenseType(""),
			wantErr: "invalid license type: enterprise",
		},
		{
			name:    "empty type: default to basic",
			args:    "",
			want:    LicenseTypeBasic,
			wantErr: "",
		},
		{
			name:    "success: platinum",
			args:    "platinum",
			want:    LicenseTypePlatinum,
			wantErr: "",
		},
		{
			name:    "success: gold",
			args:    "gold",
			want:    LicenseTypeGold,
			wantErr: "",
		},
		{
			name:    "success: trial",
			args:    "trial",
			want:    LicenseTypeTrial,
			wantErr: "",
		},
		{
			name:    "success: basic",
			args:    "basic",
			want:    LicenseTypeBasic,
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			licenseType, err := LicenseTypeFromString(tt.args)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.True(t, tt.want == licenseType)
			}
		})
	}
}

func TestLicenseType_IsGoldOrPlatinum(t *testing.T) {
	tests := []struct {
		name string
		l    LicenseType
		want bool
	}{
		{
			name: "gold",
			l:    LicenseTypeGold,
			want: true,
		},
		{
			name: "platinum",
			l:    LicenseTypePlatinum,
			want: true,
		},
		{
			name: "basic",
			l:    LicenseTypeBasic,
			want: false,
		},
		{
			name: "trial",
			l:    LicenseTypeTrial,
			want: false,
		},
		{
			name: "invalid",
			l:    LicenseType("jghk"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.IsGoldOrPlatinum(); got != tt.want {
				t.Errorf("LicenseType.IsGoldOrPlatinum() = %v, want %v", got, tt.want)
			}
		})
	}
}
