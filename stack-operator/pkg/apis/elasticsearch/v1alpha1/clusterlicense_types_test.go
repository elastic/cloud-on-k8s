package v1alpha1

import (
	"testing"
	"time"

	. "github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
)

func TestClusterLicense_IsValidAt(t *testing.T) {
	now := time.Date(2019, 01, 31, 0, 9, 0, 0, time.UTC)
	type fields struct {
		startMillis  int64
		expiryMillis int64
	}
	type args struct {
		margin SafetyMargin
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "valid license - no margin",
			fields: fields{
				startMillis:  Millis("2019-01-01"),
				expiryMillis: Millis("2019-12-31"),
			},
			want: true,
		},
		{
			name: "valid license - with margin",
			fields: fields{
				startMillis:  Millis("2019-01-01"),
				expiryMillis: Millis("2019-12-31"),
			},
			args: args{
				margin: SafetyMargin{
					ValidSince: 48 * time.Hour,
					ValidFor:   30 * 24 * time.Hour,
				},
			},
			want: true,
		},
		{
			name: "invalid license - because of margin",
			fields: fields{
				startMillis:  Millis("2019-01-30"),
				expiryMillis: Millis("2019-12-31"),
			},
			args: args{
				margin: SafetyMargin{
					ValidSince: 7 * 24 * time.Hour,
					ValidFor:   90 * 24 * time.Hour,
				},
			},
			want: false,
		},
		{
			name: "invalid license - expired",
			fields: fields{
				startMillis:  Millis("2018-01-01"),
				expiryMillis: Millis("2019-01-01"),
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
			if got := l.IsValidAt(now, tt.args.margin); got != tt.want {
				t.Errorf("ClusterLicense.IsValidAt() = %v, want %v", got, tt.want)
			}
		})
	}
}
