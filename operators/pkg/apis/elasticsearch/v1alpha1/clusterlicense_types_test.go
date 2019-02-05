// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"
	"time"

	. "github.com/elastic/k8s-operators/operators/pkg/utils/test"
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
				startMillis:  Millis("2019-01-31"),
				expiryMillis: Millis("2019-12-31"),
			},
			want: true,
		},
		{
			name: "valid license - no offset",
			fields: fields{
				startMillis:  Millis("2019-01-01"),
				expiryMillis: Millis("2019-12-31"),
			},
			want: true,
		},
		{
			name: "valid license - with offset",
			fields: fields{
				startMillis:  Millis("2019-01-01"),
				expiryMillis: Millis("2019-12-31"),
			},
			args: args{
				offset: 30 * 24 * time.Hour,
			},
			want: true,
		},
		{
			name: "invalid license - because of offset",
			fields: fields{
				startMillis:  Millis("2019-01-30"),
				expiryMillis: Millis("2019-02-28"),
			},
			args: args{
				offset: 90 * 24 * time.Hour,
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
			if got := l.IsValid(now.Add(tt.args.offset)); got != tt.want {
				t.Errorf("ClusterLicense.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
