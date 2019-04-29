// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/chrono"
)

var (
	now      = time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	gold     = v1alpha1.LicenseTypeGold
	platinum = v1alpha1.LicenseTypePlatinum
	oneMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: chrono.MustMillis("2019-02-28"),
			StartDateInMillis:  chrono.MustMillis("2019-01-01"),
		},
	}
	twoMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
			StartDateInMillis:  chrono.MustMillis("2019-01-01"),
		},
	}
	twelveMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
			StartDateInMillis:  chrono.MustMillis("2019-01-01"),
		},
	}
)

func license(l v1alpha1.ClusterLicenseSpec, t v1alpha1.LicenseType) v1alpha1.ClusterLicenseSpec {
	l.Type = t
	return l
}

func Test_bestMatchAt(t *testing.T) {
	type args struct {
		licenses []v1alpha1.EnterpriseLicense
	}
	tests := []struct {
		name      string
		args      args
		want      v1alpha1.ClusterLicenseSpec
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
				licenses: []v1alpha1.EnterpriseLicense{{
					Spec: v1alpha1.EnterpriseLicenseSpec{
						LicenseMeta: v1alpha1.LicenseMeta{
							ExpiryDateInMillis: chrono.MustMillis("2017-12-31"),
							StartDateInMillis:  chrono.MustMillis("2017-01-01"),
						},
						Type: "enterprise",
					},
				}},
			},
			wantFound: false,
			wantErr:   true,
		},
		{
			name: "error: only expired nested licenses",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2019-12-31"),
								StartDateInMillis:  chrono.MustMillis("2018-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								{
									LicenseMeta: v1alpha1.LicenseMeta{
										ExpiryDateInMillis: chrono.MustMillis("2018-12-31"),
										StartDateInMillis:  chrono.MustMillis("2018-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want:      v1alpha1.ClusterLicenseSpec{},
			wantFound: false,
			wantErr:   true,
		},
		{
			name: "success: longest valid platinum",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(oneMonth, platinum),
								license(twoMonth, platinum),
								license(twelveMonth, platinum),
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
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(oneMonth, platinum),
								license(twoMonth, platinum),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(twelveMonth, platinum),
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
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2019-03-31"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(oneMonth, gold),
								license(twoMonth, platinum),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2020-01-31"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(twoMonth, platinum),
								license(twelveMonth, gold),
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
		licenses []v1alpha1.EnterpriseLicense
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
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: chrono.MustMillis("2020-01-01"),
								StartDateInMillis:  chrono.MustMillis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								{
									Type: v1alpha1.LicenseTypePlatinum,
									LicenseMeta: v1alpha1.LicenseMeta{
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
					license: v1alpha1.ClusterLicenseSpec{
						Type: v1alpha1.LicenseTypePlatinum,
						LicenseMeta: v1alpha1.LicenseMeta{
							ExpiryDateInMillis: chrono.MustMillis("2019-02-01"),
							StartDateInMillis:  chrono.MustMillis("2019-01-01"),
						},
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
