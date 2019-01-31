package license

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
)

func Test_typeMatches(t *testing.T) {
	platinum := v1alpha1.LicenseTypePlatinum
	type args struct {
		d DesiredLicenseType
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
				d: &platinum,
				t: v1alpha1.LicenseTypePlatinum,
			},
			want: true,
		},
		{
			name: "types match: no type requested",
			args: args{
				d: nil,
				t: v1alpha1.LicenseTypeGold,
			},
			want: true,
		},
		{
			name: "types differ",
			args: args{
				d: &platinum,
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

func millis(dateStr string) int64 {
	layout := "2006-01-02"
	parsed, err := time.Parse(layout, dateStr)
	if err != nil {
		panic(fmt.Sprintf("incorrect test setup can't parse date %v", err))
	}
	return parsed.UnixNano() / int64(time.Millisecond)
}

var (
	now      = time.Date(2019, 01, 31, 0, 0, 0, 0, time.UTC)
	gold     = v1alpha1.LicenseTypeGold
	platinum = v1alpha1.LicenseTypePlatinum
	oneMonth = v1alpha1.ClusterLicense{
		Spec: v1alpha1.ClusterLicenseSpec{
			ExpiryDateInMillis: millis("2019-02-28"),
			StartDateInMillis:  millis("2019-01-01"),
		},
	}
	twoMonth = v1alpha1.ClusterLicense{
		Spec: v1alpha1.ClusterLicenseSpec{
			ExpiryDateInMillis: millis("2019-03-31"),
			StartDateInMillis:  millis("2019-01-01"),
		},
	}
	twelveMonth = v1alpha1.ClusterLicense{
		Spec: v1alpha1.ClusterLicenseSpec{
			ExpiryDateInMillis: millis("2020-01-31"),
			StartDateInMillis:  millis("2019-01-01"),
		},
	}
)

func license(l v1alpha1.ClusterLicense, t v1alpha1.LicenseType) v1alpha1.ClusterLicense {
	l.Spec.Type = t
	return l
}

func Test_bestMatchAt(t *testing.T) {
	type args struct {
		licenses       []v1alpha1.EnterpriseLicense
		desiredLicense DesiredLicenseType
	}
	tests := []struct {
		name    string
		args    args
		want    v1alpha1.ClusterLicense
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
						ExpiryDateInMillis: millis("2017-12-31"),
						StartDateInMillis:  millis("2017-01-01"),
						Type:               "enterprise",
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

							ExpiryDateInMillis: millis("2019-12-31"),
							StartDateInMillis:  millis("2018-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								{

									Spec: v1alpha1.ClusterLicenseSpec{
										ExpiryDateInMillis: millis("2018-12-31"),
										StartDateInMillis:  millis("2018-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want:    v1alpha1.ClusterLicense{},
			wantErr: true,
		},
		{
			name: "success: longest valid platinum",
			args: args{
				licenses: []v1alpha1.EnterpriseLicense{
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							ExpiryDateInMillis: millis("2020-01-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
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
							ExpiryDateInMillis: millis("2019-03-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								license(oneMonth, platinum),
								license(twoMonth, platinum),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							ExpiryDateInMillis: millis("2020-01-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
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
							ExpiryDateInMillis: millis("2019-03-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								license(oneMonth, gold),
								license(twoMonth, platinum),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							ExpiryDateInMillis: millis("2020-01-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								license(twoMonth, gold),
								license(twelveMonth, platinum),
							},
						},
					},
				},
				desiredLicense: &gold,
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
							ExpiryDateInMillis: millis("2019-03-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								license(oneMonth, gold),
								license(twoMonth, platinum),
							},
						},
					},
					{
						Spec: v1alpha1.EnterpriseLicenseSpec{
							ExpiryDateInMillis: millis("2020-01-31"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
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
			got, err := bestMatchAt(now, tt.args.licenses, tt.args.desiredLicense)
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
		licenseType DesiredLicenseType
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
							ExpiryDateInMillis: millis("2020-01-01"),
							StartDateInMillis:  millis("2019-01-01"),
							ClusterLicenses: []v1alpha1.ClusterLicense{
								{
									Spec: v1alpha1.ClusterLicenseSpec{
										Type:               v1alpha1.LicenseTypePlatinum,
										ExpiryDateInMillis: millis("2019-02-01"),
										StartDateInMillis:  millis("2019-01-01"),
									},
								},
							},
						},
					},
				},
			},
			want: []licenseWithTimeLeft{
				{
					l: v1alpha1.ClusterLicense{
						Spec: v1alpha1.ClusterLicenseSpec{
							Type:               v1alpha1.LicenseTypePlatinum,
							ExpiryDateInMillis: millis("2019-02-01"),
							StartDateInMillis:  millis("2019-01-01"),
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
