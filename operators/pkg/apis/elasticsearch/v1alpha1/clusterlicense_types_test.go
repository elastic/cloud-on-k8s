// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/chrono"
	"github.com/stretchr/testify/require"
)

func TestClusterLicense_IsValidAt(t *testing.T) {
	now := time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	type fields struct {
		startMillis  int64
		expiryMillis int64
	}
	type args struct {
		offset time.Duration
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "valid license - starts now",
			fields: fields{
				startMillis:  chrono.MustMillis("2019-01-31"),
				expiryMillis: chrono.MustMillis("2019-12-31"),
			},
			want: true,
		},
		{
			name: "valid license - no offset",
			fields: fields{
				startMillis:  chrono.MustMillis("2019-01-01"),
				expiryMillis: chrono.MustMillis("2019-12-31"),
			},
			want: true,
		},
		{
			name: "valid license - with offset",
			fields: fields{
				startMillis:  chrono.MustMillis("2019-01-01"),
				expiryMillis: chrono.MustMillis("2019-12-31"),
			},
			args: args{
				offset: 30 * 24 * time.Hour,
			},
			want: true,
		},
		{
			name: "invalid license - because of offset",
			fields: fields{
				startMillis:  chrono.MustMillis("2019-01-30"),
				expiryMillis: chrono.MustMillis("2019-02-28"),
			},
			args: args{
				offset: 90 * 24 * time.Hour,
			},
			want: false,
		},
		{
			name: "invalid license - expired",
			fields: fields{
				startMillis:  chrono.MustMillis("2018-01-01"),
				expiryMillis: chrono.MustMillis("2019-01-01"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := ClusterLicense{
				Spec: ClusterLicenseSpec{
					LicenseMeta: LicenseMeta{
						ExpiryDateInMillis: tt.fields.expiryMillis,
						StartDateInMillis:  tt.fields.startMillis,
					},
				},
			}
			if got := l.IsValid(now.Add(tt.args.offset)); got != tt.want {
				t.Errorf("ClusterLicense.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
