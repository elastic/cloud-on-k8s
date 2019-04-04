/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package mutation

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/magiconair/properties/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_toMillis(t *testing.T) {
	type args struct {
		t time.Time
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "turnes time into unix milliseconds",
			args: args{
				t: time.Date(2019, 01, 22, 0, 0, 0, 0, time.UTC),
			},
			want: 1548115200000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toMillis(tt.args.t); got != tt.want {
				t.Errorf("toMillis() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPopulateTrialLicense(t *testing.T) {
	type args struct {
		l *v1alpha1.EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(v1alpha1.EnterpriseLicense)
		wantErr    bool
	}{
		{
			name:    "nil FAIL",
			args:    args{},
			wantErr: true,
		},
		{
			name: "non-trial FAIL",
			args: args{
				l: &v1alpha1.EnterpriseLicense{
					Spec: v1alpha1.EnterpriseLicenseSpec{
						Type: v1alpha1.LicenseTypeEnterprise,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "trial license OK",
			args: args{
				l: &v1alpha1.EnterpriseLicense{
					ObjectMeta: v1.ObjectMeta{
						UID: "this-would-come-from-the-api-server",
					},
					Spec: v1alpha1.EnterpriseLicenseSpec{
						Type: v1alpha1.LicenseTypeEnterpriseTrial,
					},
				},
			},
			assertions: func(l v1alpha1.EnterpriseLicense) {
				require.NoError(t, l.IsMissingFields())
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PopulateTrialLicense(tt.args.l); (err != nil) != tt.wantErr {
				t.Errorf("PopulateTrialLicense() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.assertions != nil {
				tt.assertions(*tt.args.l)
			}
		})
	}
}

func TestStartTrial(t *testing.T) {
	dateFixture := time.Date(2019, 01, 22, 0, 0, 0, 0, time.UTC)
	type args struct {
		start time.Time
		l     *v1alpha1.EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(v1alpha1.EnterpriseLicense)
	}{
		{
			name: "trial is 30 days",
			args: args{
				start: dateFixture,
				l:     &v1alpha1.EnterpriseLicense{},
			},
			assertions: func(license v1alpha1.EnterpriseLicense) {
				assert.Equal(t, license.ExpiryDate().UTC(), time.Date(2019, 02, 21, 0, 0, 0, 0, time.UTC))
				assert.Equal(t, license.StartDate().UTC(), dateFixture)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			StartTrial(tt.args.l, tt.args.start)
		})
		if tt.assertions != nil {
			tt.assertions(*tt.args.l)
		}
	}
}
