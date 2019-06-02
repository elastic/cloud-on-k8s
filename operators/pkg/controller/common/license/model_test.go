/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"encoding/json"
	"io/ioutil"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
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
			l := SourceEnterpriseLicense{
				Data: SourceLicenseData{
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

var expectedLicenseSpec = SourceEnterpriseLicense{
	Data: SourceLicenseData{
		UID:                "840F0DB6-1906-452E-98C7-6F94E6012CD7",
		IssueDateInMillis:  1548115200000,
		ExpiryDateInMillis: 1561247999999,
		IssuedTo:           "test org",
		Issuer:             "test issuer",
		StartDateInMillis:  1548115200000,
		Type:               "enterprise",
		MaxInstances:       40,
		Signature:          "test signature",
		ClusterLicenses: []SourceClusterLicense{
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

func Test_unmarshalModel(t *testing.T) {
	require.NoError(t, apis.AddToScheme(scheme.Scheme))
	type args struct {
		licenseFile string
	}
	tests := []struct {
		name      string
		args      args
		wantErr   bool
		assertion func(el SourceEnterpriseLicense)
	}{
		{
			name: "invalid input: FAIL",
			args: args{
				licenseFile: "testdata/test-error.json",
			},
			wantErr: true,
		},
		{
			name: "valid input: OK",
			args: args{
				licenseFile: "testdata/test-license.json",
			},
			wantErr: false,
			assertion: func(el SourceEnterpriseLicense) {
				if diff := deep.Equal(el, expectedLicenseSpec); diff != nil {
					t.Error(diff)
				}
			},
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			var license SourceEnterpriseLicense
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
