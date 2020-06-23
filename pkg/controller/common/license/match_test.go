// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
)

var (
	now        = time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	gold       = client.ElasticsearchLicenseTypeGold
	platinum   = client.ElasticsearchLicenseTypePlatinum
	trial      = client.ElasticsearchLicenseTypeTrial
	enterprise = client.ElasticsearchLicenseTypeEnterprise
	expired    = client.License{
		ExpiryDateInMillis: chrono.MustMillis("2018-12-31"),
		StartDateInMillis:  chrono.MustMillis("2018-01-01"),
	}
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

func license(l client.License, t client.ElasticsearchLicenseType) client.License {
	l.Type = string(t)
	return l
}

func noopFilter(_ EnterpriseLicense) (bool, error) {
	return true, nil
}

func Test_bestMatchAt(t *testing.T) {
	type args struct {
		licenses   []EnterpriseLicense
		minVersion version.Version
	}
	tests := []struct {
		name      string
		args      args
		want      client.License
		wantFound bool
	}{
		{
			name:      "no result: no licenses",
			wantFound: false,
		},
		{
			name: "no result: only expired enterprise license",
			args: args{
				licenses: []EnterpriseLicense{{
					License: LicenseSpec{
						ExpiryDateInMillis: chrono.MustMillis("2017-12-31"),
						StartDateInMillis:  chrono.MustMillis("2017-01-01"),
						Type:               "enterprise",
					},
				}},
			},
			wantFound: false,
		},
		{
			name: "no result: only expired nested licenses",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2019-12-31"),
							StartDateInMillis:  chrono.MustMillis("2018-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(expired, platinum)},
							},
						},
					},
				},
			},
			want:      client.License{},
			wantFound: false,
		},
		{
			name: "success: prefer external trial over expired platinum",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(expired, platinum)},
								{License: license(expired, platinum)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
							Issuer:             "API",
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(oneMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(oneMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: prefer eck managed trial over expired platinum",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(expired, platinum)},
								{License: license(expired, platinum)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
						},
					},
				},
			},
			want:      license(client.License{}, trial),
			wantFound: true,
		},
		{
			name: "success: prefer platinum over external trial",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterprise,
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twelveMonth, platinum)},
								{License: license(twoMonth, gold)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
							Issuer:             "API",
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twelveMonth, trial)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: prefer platinum over eck managed trial",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterprise,
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twelveMonth, platinum)},
								{License: license(twoMonth, gold)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: prefer external trial over eck managed trial",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							Type:               LicenseTypeEnterpriseTrial,
							Issuer:             "API",
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twelveMonth, platinum)},
								{License: license(twoMonth, gold)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: longest valid platinum",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
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
		},
		{
			name: "success: mixed platinum/enterprise pre 7.8.1",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(oneMonth, enterprise)},
								{License: license(twoMonth, platinum)},
								{License: license(twelveMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: mixed platinum/enterprise post 7.8.1",
			args: args{
				minVersion: version.MustParse("7.8.1"),
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(oneMonth, enterprise)},
								{License: license(twoMonth, platinum)},
								{License: license(twelveMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(oneMonth, enterprise),
			wantFound: true,
		},
		{
			name: "success: longest valid from multiple enterprise licenses",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(oneMonth, platinum)},
								{License: license(twoMonth, platinum)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twelveMonth, platinum)},
							},
						},
					},
				},
			},
			want:      license(twelveMonth, platinum),
			wantFound: true,
		},
		{
			name: "success: best license",
			args: args{
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(oneMonth, gold)},
								{License: license(twoMonth, platinum)},
							},
						},
					},
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{License: license(twoMonth, platinum)},
								{License: license(twelveMonth, gold)},
							},
						},
					},
				},
			},
			want:      license(twoMonth, platinum),
			wantFound: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, _, found := bestMatchAt(now, &tt.args.minVersion, tt.args.licenses, noopFilter)
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
		minVersion version.Version
		licenses   []EnterpriseLicense
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
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-01"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{
									License: client.License{
										Type:               string(client.ElasticsearchLicenseTypePlatinum),
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
						Type:               string(client.ElasticsearchLicenseTypePlatinum),
						ExpiryDateInMillis: chrono.MustMillis("2019-02-01"),
						StartDateInMillis:  chrono.MustMillis("2019-01-01"),
					},
					remaining: 24 * time.Hour,
				},
			},
		},
		{
			name: "matching is version specific: pre-7.8.1",
			args: args{
				minVersion: version.MustParse("7.5.0"),
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-01"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{
									License: client.License{
										Type:               string(client.ElasticsearchLicenseTypeEnterprise),
										ExpiryDateInMillis: chrono.MustMillis("2019-02-01"),
										StartDateInMillis:  chrono.MustMillis("2019-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want: []licenseWithTimeLeft{},
		},
		{
			name: "matching is version specific: post-7.8.1",
			args: args{
				minVersion: version.MustParse("7.8.1"),
				licenses: []EnterpriseLicense{
					{
						License: LicenseSpec{
							ExpiryDateInMillis: chrono.MustMillis("2020-01-01"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							ClusterLicenses: []ElasticsearchLicense{
								{
									License: client.License{
										Type:               string(client.ElasticsearchLicenseTypeEnterprise),
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
						Type:               string(client.ElasticsearchLicenseTypeEnterprise),
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
			if got := filterValid(now, &tt.args.minVersion, tt.args.licenses, noopFilter); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterValidForType expected %v, got %v", tt.want, got)
			}
		})
	}
}
