// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
)

var (
	now      = time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	gold     = v1alpha1.LicenseTypeGold
	platinum = v1alpha1.LicenseTypePlatinum
	oneMonth = client.License{
		ExpiryDateInMillis: chrono.MustMillis("2019-02-28"),
		StartDateInMillis:  chrono.MustMillis("2019-01-01"),
	}
	twoMonth = client.License{
		ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
		StartDateInMillis:  chrono.MustMillis("2019-01-01"),
	}
	twelveMonth = client.License{
		ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
		StartDateInMillis:  chrono.MustMillis("2019-01-01"),
	}
)

func license(l client.License, t v1alpha1.LicenseType) client.License {
	l.Type = string(t)
	return l
}

func Test_bestMatchAt(t *testing.T) {
	type args struct {
		licenses []SourceEnterpriseLicense
	}
	tests := []struct {
		name      string
		args      args
		want      client.License
		wantFound bool
		wantErr   bool
	}{
		{
			name:      "error: no licenses",
			wantFound: false,
			wantErr:   false,
		},
		{
			name: "error: only expired enterprise license",
			args: args{
				licenses: []SourceEnterpriseLicense{{
					Data: SourceLicenseData{
						ExpiryDateInMillis: chrono.MustMillis("2017-12-31"),
						StartDateInMillis:  chrono.MustMillis("2017-01-01"),
						Type:               "enterprise",
					},
				}},
			},
			wantFound: false,
			wantErr:   true,
		},
		{
			name: "error: only expired nested licenses",
			args: args{
				licenses: []SourceEnterpriseLicense{
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2019-12-31"),
							StartDateInMillis:  chrono.MustMillis("2018-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{
									License: client.License{
										ExpiryDateInMillis: chrono.MustMillis("2018-12-31"),
										StartDateInMillis:  chrono.MustMillis("2018-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want:      client.License{},
			wantFound: false,
			wantErr:   true,
		},
		{
			name: "success: longest valid platinum",
			args: args{
				licenses: []SourceEnterpriseLicense{
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{License: license(oneMonth, platinum)},
								{License: license(twoMonth, platinum)},
								{License: license(twelveMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "success: longest valid from multiple enterprise licenses",
			args: args{
				licenses: []SourceEnterpriseLicense{
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{License: license(oneMonth, platinum)},
								{License: license(twoMonth, platinum)},
							},
						},
					},
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{License: license(twelveMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
			wantErr:   false,
		},
		{
			name: "success: best license",
			args: args{
				licenses: []SourceEnterpriseLicense{
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{License: license(oneMonth, gold)},
								{License: license(twoMonth, platinum)},
							},
						},
					},
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{License: license(twoMonth, platinum)},
								{License: license(twelveMonth, gold)},
							},
						},
					},
				},
			},
			want:      license(twoMonth, platinum),
			wantFound: true,
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, found, err := bestMatchAt(now, tt.args.licenses)
			if (err != nil) != tt.wantErr {
				t.Errorf("bestMatchAt() error = %v, wantErr %v, got %v", err, tt.wantErr, got)
				return
			}
			if tt.wantFound != found {
				t.Errorf("bestMatchAt() found = %v, want %v", found, tt.wantFound)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bestMatchAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_filterValidForType(t *testing.T) {
	type args struct {
		licenses []SourceEnterpriseLicense
	}
	tests := []struct {
		name string
		args args
		want []licenseWithTimeLeft
	}{
		{
			name: "no licenses",
			args: args{},
			want: []licenseWithTimeLeft{},
		},
		{
			name: "single match",
			args: args{
				licenses: []SourceEnterpriseLicense{
					{
						Data: SourceLicenseData{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-01"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []SourceClusterLicense{
								{
									License: client.License{
										Type:               string(v1alpha1.LicenseTypePlatinum),
										ExpiryDateInMillis: chrono.MustMillis("2019-02-01"),
										StartDateInMillis:  chrono.MustMillis("2019-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want: []licenseWithTimeLeft{
				{
					license: client.License{
						Type:               string(v1alpha1.LicenseTypePlatinum),
						ExpiryDateInMillis: chrono.MustMillis("2019-02-01"),
						StartDateInMillis:  chrono.MustMillis("2019-01-01"),
					},
					remaining: 24 * time.Hour,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterValid(now, tt.args.licenses); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterValidForType expected %v, got %v", tt.want, got)
			}
		})
	}
}
