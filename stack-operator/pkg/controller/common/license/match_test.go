package license

import (
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	. "github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
)

func Test_typeMatches(t *testing.T) {
	platinum := v1alpha1.LicenseTypePlatinum
	type args struct {
		d v1alpha1.LicenseType
		t v1alpha1.LicenseType
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "types match",
			args: args{
				d: platinum,
				t: v1alpha1.LicenseTypePlatinum,
			},
			want: true,
		},
		{
			name: "types match: no type requested",
			args: args{
				t: v1alpha1.LicenseTypeGold,
			},
			want: true,
		},
		{
			name: "types differ",
			args: args{
				d: platinum,
				t: v1alpha1.LicenseTypeGold,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := typeMatches(tt.args.d, tt.args.t); got != tt.want {
				t.Errorf("typeMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

var (
	now      = time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	gold     = v1alpha1.LicenseTypeGold
	platinum = v1alpha1.LicenseTypePlatinum
	standard = v1alpha1.LicenseTypeStandard
	oneMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: Millis("2019-02-28"),
			StartDateInMillis:  Millis("2019-01-01"),
		},
	}
	twoMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: Millis("2019-03-31"),
			StartDateInMillis:  Millis("2019-01-01"),
		},
	}
	twelveMonth = v1alpha1.ClusterLicenseSpec{
		LicenseMeta: v1alpha1.LicenseMeta{
			ExpiryDateInMillis: Millis("2020-01-31"),
			StartDateInMillis:  Millis("2019-01-01"),
		},
	}
)

func license(l v1alpha1.ClusterLicenseSpec, t v1alpha1.LicenseType) v1alpha1.ClusterLicenseSpec {
	l.Type = t
	return l
}

func Test_bestMatchAt(t *testing.T) {
	type args struct {
		licenses       []v1alpha1.EnterpriseLicense
		desiredLicense v1alpha1.LicenseType
	}
	tests := []struct {
		name    string
		args    args
		want    v1alpha1.ClusterLicenseSpec
		wantErr bool
	}{
		{
			name:    "error: no licenses",
			wantErr: true,
		},
		{
			name: "error: only expired enterprise license",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{{
					Spec: v1alpha1.EnterpriseLicenseSpec{
						LicenseMeta: v1alpha1.LicenseMeta{
							ExpiryDateInMillis: Millis("2017-12-31"),
							StartDateInMillis:  Millis("2017-01-01"),
						},
						Type: "enterprise",
					},
				}},
			},
			wantErr: true,
		},
		{
			name: "error: only expired nested licenses",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2019-12-31"),
								StartDateInMillis:  Millis("2018-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								{
									LicenseMeta: v1alpha1.LicenseMeta{
										ExpiryDateInMillis: Millis("2018-12-31"),
										StartDateInMillis:  Millis("2018-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want:    v1alpha1.ClusterLicenseSpec{},
			wantErr: true,
		},
		{
			name: "success: longest valid platinum",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2020-01-31"),
								StartDateInMillis:  Millis("2019-01-01"),
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
			want:    license(twelveMonth, platinum),
			wantErr: false,
		},
		{
			name: "success: longest valid from multiple enterprise licenses",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2019-03-31"),
								StartDateInMillis:  Millis("2019-01-01"),
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
								ExpiryDateInMillis: Millis("2020-01-31"),
								StartDateInMillis:  Millis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(twelveMonth, platinum),
							},
						},
					},
				},
			},
			want:    license(twelveMonth, platinum),
			wantErr: false,
		},
		{
			name: "success: longest valid of specific type",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2019-03-31"),
								StartDateInMillis:  Millis("2019-01-01"),
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
								ExpiryDateInMillis: Millis("2020-01-31"),
								StartDateInMillis:  Millis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(twoMonth, gold),
								license(twelveMonth, platinum),
							},
						},
					},
				},
				desiredLicense: gold,
			},
			want:    license(twoMonth, gold),
			wantErr: false,
		},
		{
			name: "success: best license when type not specified",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2019-03-31"),
								StartDateInMillis:  Millis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(oneMonth, gold),
								license(twoMonth, platinum),
								license(twelveMonth, standard),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							LicenseMeta: v1alpha1.LicenseMeta{
								ExpiryDateInMillis: Millis("2020-01-31"),
								StartDateInMillis:  Millis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								license(twoMonth, platinum),
								license(twelveMonth, gold),
							},
						},
					},
				},
			},
			want:    license(twoMonth, platinum),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := bestMatchAt(now, tt.args.licenses, tt.args.desiredLicense)
			if (err != nil) != tt.wantErr {
				t.Errorf("bestMatchAt() error = %v, wantErr %v, got %v", err, tt.wantErr, got)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bestMatchAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_filterValidForType(t *testing.T) {
	type args struct {
		licenseType v1alpha1.LicenseType
		licenses    []v1alpha1.EnterpriseLicense
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
								ExpiryDateInMillis: Millis("2020-01-01"),
								StartDateInMillis:  Millis("2019-01-01"),
							},
							ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
								{
									Type: v1alpha1.LicenseTypePlatinum,
									LicenseMeta: v1alpha1.LicenseMeta{
										ExpiryDateInMillis: Millis("2019-02-01"),
										StartDateInMillis:  Millis("2019-01-01"),
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
							ExpiryDateInMillis: Millis("2019-02-01"),
							StartDateInMillis:  Millis("2019-01-01"),
						},
					},
					remaining: 24 * time.Hour,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filterValidForType(tt.args.licenseType, now, tt.args.licenses); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterValidForType expected %v, got %v", tt.want, got)
			}
		})
	}
}
