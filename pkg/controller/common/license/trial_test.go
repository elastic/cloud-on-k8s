// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"crypto/x509"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestInitTrialLicense(t *testing.T) {
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
		state   TrialState
		args    args
		want    func(*EnterpriseLicense)
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
			want: func(l *EnterpriseLicense) {
				require.Equal(t, *l, EnterpriseLicense{})
			},
			wantErr: true,
		},
		{
			name: "successful trial start",
			state: func() TrialState {
				state, err := NewTrialState()
				require.NoError(t, err)
				return state
			}(),
			args: args{
				l: &licenseFixture,
			},
			want: func(l *EnterpriseLicense) {
				require.NoError(t, l.IsMissingFields())
			},
			wantErr: false,
		},
		{
			name: "not in activation state",
			args: args{
				l: &licenseFixture,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.InitTrialLicense(context.Background(), tt.args.l)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitTrial() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				tt.want(tt.args.l)
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
		{
			// technically this code path should not be possible: we use the new type when creating a new trial
			// and we don't repopulate existing licenses
			name: "legacy trial still supported",
			args: args{
				l: &EnterpriseLicense{
					License: LicenseSpec{
						Type: LicenseTypeLegacyTrial,
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

func TestNewTrialStateFromStatus(t *testing.T) {
	key, err := newTrialKey()
	require.NoError(t, err)

	keySerialized, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	type args struct {
		trialStatus v1.Secret
	}
	tests := []struct {
		name    string
		args    args
		want    func(TrialState)
		wantErr bool
	}{
		{
			name: "reconstructs state",
			args: args{
				trialStatus: v1.Secret{
					Data: map[string][]byte{
						TrialPubkeyKey: keySerialized,
					},
				},
			},
			want: func(s TrialState) {
				require.True(t, s.IsTrialStarted())
				require.True(t, reflect.DeepEqual(s, TrialState{
					publicKey: &key.PublicKey,
				}))
			},
			wantErr: false,
		},
		{
			name: "error on garbage status",
			args: args{
				trialStatus: v1.Secret{
					Data: map[string][]byte{
						TrialPubkeyKey: []byte("foo"),
					},
				},
			},
			wantErr: true,
			want: func(state TrialState) {
				require.False(t, state.IsTrialStarted())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTrialStateFromStatus(tt.args.trialStatus)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTrialStateFromStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				tt.want(got)
			}
		})
	}
}

func TestExpectedTrialStatus(t *testing.T) {
	sampleKey, err := newTrialKey()
	require.NoError(t, err)
	pubKey, err := x509.MarshalPKIXPublicKey(&sampleKey.PublicKey)
	require.NoError(t, err)

	type args struct {
		state TrialState
	}
	tests := []struct {
		name    string
		args    args
		want    v1.Secret
		wantErr bool
	}{
		{
			name: "status during activation",
			args: args{
				state: TrialState{
					publicKey:  &sampleKey.PublicKey,
					privateKey: sampleKey,
				},
			},
			want: v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      TrialStatusSecretKey,
					Annotations: map[string]string{
						TrialLicenseSecretName:      "name",
						TrialLicenseSecretNamespace: "ns",
					},
				},
				Data: map[string][]byte{
					TrialPubkeyKey:     pubKey,
					TrialActivationKey: []byte("true"),
				},
			},
			wantErr: false,
		},
		{
			name: "status after activation",
			args: args{
				state: TrialState{
					publicKey: &sampleKey.PublicKey,
				},
			},
			want: v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      TrialStatusSecretKey,
					Annotations: map[string]string{
						TrialLicenseSecretName:      "name",
						TrialLicenseSecretNamespace: "ns",
					},
				},
				Data: map[string][]byte{
					TrialPubkeyKey: pubKey,
				},
			},
			wantErr: false,
		},
		{
			name: "with empty trial state",
			args: args{
				state: TrialState{},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpectedTrialStatus("ns", types.NamespacedName{
				Namespace: "ns",
				Name:      "name",
			}, tt.args.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpectedTrialStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpectedTrialStatus() got = %v, want %v", got, tt.want)
			}
		})
	}
}
