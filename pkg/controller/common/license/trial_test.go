// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTrial(t *testing.T) {
	licenseFixture := EnterpriseLicense{
		License: LicenseSpec{
			Type: LicenseTypeEnterpriseTrial,
		},
	}
	type args struct {
		l *EnterpriseLicense
	}
	tests := []struct {
		name    string
		args    args
		want    func(*EnterpriseLicense, *rsa.PublicKey)
		wantErr bool
	}{
		{
			name: "nil license",
			args: args{
				l: nil,
			},
			wantErr: true,
		},
		{
			name: "not a trial license",
			args: args{
				l: &EnterpriseLicense{},
			},
			want: func(l *EnterpriseLicense, k *rsa.PublicKey) {
				require.Equal(t, *l, EnterpriseLicense{})
				require.Nil(t, k)
			},
			wantErr: true,
		},
		{
			name: "successful trial start",
			args: args{
				l: &licenseFixture,
			},
			want: func(l *EnterpriseLicense, k *rsa.PublicKey) {
				require.NotNil(t, k)
				require.NoError(t, l.IsMissingFields())
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rnd := rand.Reader
			tmpPrivKey, err := rsa.GenerateKey(rnd, 2048)
			err = InitTrial(
				tmpPrivKey,
				tt.args.l,
			)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitTrial() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestPopulateTrialLicense(t *testing.T) {
	type args struct {
		l *EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(EnterpriseLicense)
		wantErr    bool
	}{
		{
			name: "non-trial FAIL",
			args: args{
				l: &EnterpriseLicense{
					License: LicenseSpec{
						Type: LicenseTypeEnterprise,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "trial license OK",
			args: args{
				l: &EnterpriseLicense{
					License: LicenseSpec{
						Type: LicenseTypeEnterpriseTrial,
					},
				},
			},
			assertions: func(l EnterpriseLicense) {
				require.NoError(t, l.IsMissingFields())
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := populateTrialLicense(tt.args.l); (err != nil) != tt.wantErr {
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
		l     *EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(EnterpriseLicense)
	}{
		{
			name: "trial is 30 days",
			args: args{
				start: dateFixture,
				l:     &EnterpriseLicense{},
			},
			assertions: func(license EnterpriseLicense) {
				assert.Equal(t, license.ExpiryTime().UTC(), time.Date(2019, 02, 21, 0, 0, 0, 0, time.UTC))
				assert.Equal(t, license.StartTime().UTC(), dateFixture)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setStartAndExpiry(tt.args.l, tt.args.start)
		})
		if tt.assertions != nil {
			tt.assertions(*tt.args.l)
		}
	}
}
