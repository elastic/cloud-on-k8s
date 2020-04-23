// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
)

func TestLicense_IsValidAt(t *testing.T) {
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
			l := EnterpriseLicense{
				License: LicenseSpec{
					ExpiryDateInMillis: tt.fields.expiryMillis,
					StartDateInMillis:  tt.fields.startMillis,
				},
			}
			if got := l.IsValid(now.Add(tt.args.offset)); got != tt.want {
				t.Errorf("ClusterLicense.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

var expectedLicenseSpec = EnterpriseLicense{
	License: LicenseSpec{
		UID:                "840F0DB6-1906-452E-98C7-6F94E6012CD7",
		IssueDateInMillis:  1548115200000,
		ExpiryDateInMillis: 1561247999999,
		IssuedTo:           "test org",
		Issuer:             "test issuer",
		StartDateInMillis:  1548115200000,
		Type:               "enterprise",
		MaxInstances:       40,
		Signature:          "test signature",
		ClusterLicenses: []ElasticsearchLicense{
			{
				License: client.License{
					UID:                "73117B2A-FEEA-4FEC-B8F6-49D764E9F1DA",
					IssueDateInMillis:  1548115200000,
					ExpiryDateInMillis: 1561247999999,
					IssuedTo:           "test org",
					Issuer:             "test issuer",
					StartDateInMillis:  1548115200000,
					MaxNodes:           100,
					Type:               "gold",
					Signature:          "test signature gold",
				},
			},
			{
				License: client.License{
					UID:                "57E312E2-6EA0-49D0-8E65-AA5017742ACF",
					IssueDateInMillis:  1548115200000,
					ExpiryDateInMillis: 1561247999999,
					IssuedTo:           "test org",
					Issuer:             "test issuer",
					StartDateInMillis:  1548115200000,
					MaxNodes:           100,
					Type:               "platinum",
					Signature:          "test signature platinum",
				},
			},
		},
	},
}

var expectedLicenseSpecV4 = EnterpriseLicense{
	License: LicenseSpec{
		UID:                "840F0DB6-1906-452E-98C7-6F94E6012CD7",
		IssueDateInMillis:  1548115200000,
		ExpiryDateInMillis: 1561247999999,
		IssuedTo:           "test org",
		Issuer:             "test issuer",
		StartDateInMillis:  1548115200000,
		Type:               "enterprise",
		MaxResourceUnits:   20,
		Signature:          "test signature",
		ClusterLicenses: []ElasticsearchLicense{
			{
				License: client.License{
					UID:                "73117B2A-FEEA-4FEC-B8F6-49D764E9F1DA",
					IssueDateInMillis:  1548115200000,
					ExpiryDateInMillis: 1561247999999,
					IssuedTo:           "test org",
					Issuer:             "test issuer",
					StartDateInMillis:  1548115200000,
					MaxNodes:           100,
					Type:               "platinum",
					Signature:          "test signature platinum",
				},
			},
			{
				License: client.License{
					UID:                "57E312E2-6EA0-49D0-8E65-AA5017742ACF",
					IssueDateInMillis:  1548115200000,
					ExpiryDateInMillis: 1561247999999,
					IssuedTo:           "test org",
					Issuer:             "test issuer",
					StartDateInMillis:  1548115200000,
					MaxResourceUnits:   50,
					Type:               "enterprise",
					Signature:          "test signature enterprise",
				},
			},
		},
	},
}

func Test_unmarshalModel(t *testing.T) {
	controllerscheme.SetupScheme()
	type args struct {
		licenseFile string
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		assertion func(el EnterpriseLicense)
	}{
		{
			name: "invalid input: FAIL",
			args: args{
				licenseFile: "testdata/test-error.json",
			},
			wantErr: true,
		},
		{
			name: "valid input: license v3 OK",
			args: args{
				licenseFile: "testdata/test-license.json",
			},
			wantErr: false,
			assertion: func(el EnterpriseLicense) {
				if diff := deep.Equal(el, expectedLicenseSpec); diff != nil {
					t.Error(diff)
				}
			},
		},
		{
			name: "valid input: license v4 OK",
			args: args{
				licenseFile: "testdata/test-license-v4.json",
			},
			wantErr: false,
			assertion: func(el EnterpriseLicense) {
				if diff := deep.Equal(el, expectedLicenseSpecV4); diff != nil {
					t.Error(diff)
				}
			},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			var license EnterpriseLicense
			bytes, err := ioutil.ReadFile(tt.args.licenseFile)
			require.NoError(t, err)

			if err := json.Unmarshal(bytes, &license); (err != nil) != tt.wantErr {
				t.Errorf("extractTransformLoadLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.assertion != nil {
				tt.assertion(license)
			}
		})
	}
}

func TestEnterpriseLicense_IsECKManagedTrial(t *testing.T) {
	type fields struct {
		License LicenseSpec
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name: "true: type trial and expected issuer ",
			fields: fields{
				License: LicenseSpec{
					Type:   LicenseTypeEnterpriseTrial,
					Issuer: ECKLicenseIssuer,
				},
			},
			want: true,
		},
		{
			name: "true: type legacy trial and expected issuer",
			fields: fields{
				License: LicenseSpec{
					Type:   LicenseTypeLegacyTrial,
					Issuer: ECKLicenseIssuer,
				},
			},
			want: true,
		},
		{
			name: "false: wrong type",
			fields: fields{
				License: LicenseSpec{
					Type:   LicenseTypeEnterprise,
					Issuer: ECKLicenseIssuer,
				},
			},
			want: false,
		},
		{
			name: "false: wrong issuer",
			fields: fields{
				License: LicenseSpec{
					Type:   LicenseTypeEnterpriseTrial,
					Issuer: "API",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := EnterpriseLicense{
				License: tt.fields.License,
			}
			if got := l.IsECKManagedTrial(); got != tt.want {
				t.Errorf("IsECKManagedTrial() = %v, want %v", got, tt.want)
			}
		})
	}
}
