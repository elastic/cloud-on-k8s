// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/rsa"
	"testing"
	"time"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type failingClient struct {
	k8s.Client
}

func (failingClient) Create(o runtime.Object) error {
	return errors.New("boom")
}

func TestInitTrial(t *testing.T) {
	require.NoError(t, estype.AddToScheme(scheme.Scheme))

	licenseFixture := estype.EnterpriseLicense{
		Spec: estype.EnterpriseLicenseSpec{
			Type: estype.LicenseTypeEnterpriseTrial,
		},
	}
	type args struct {
		c k8s.Client
		l *estype.EnterpriseLicense
	}
	tests := []struct {
		name    string
		args    args
		want    func(*estype.EnterpriseLicense, *rsa.PublicKey)
		wantErr bool
	}{
		{
			name: "nil license",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				l: nil,
			},
			wantErr: true,
		},
		{
			name: "failing client",
			args: args{
				c: failingClient{},
				l: &estype.EnterpriseLicense{
					ObjectMeta: v1.ObjectMeta{
						Name: "failing client test",
					},
					Spec: estype.EnterpriseLicenseSpec{
						Type: estype.LicenseTypeEnterpriseTrial,
					},
				},
			},
			want: func(_ *estype.EnterpriseLicense, key *rsa.PublicKey) {
				require.Nil(t, key)
			},
			wantErr: true,
		},
		{
			name: "deleted/non-existing license",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				l: &licenseFixture,
			},
			wantErr: true,
		},
		{
			name: "not a trial license",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				l: &estype.EnterpriseLicense{},
			},
			want: func(l *estype.EnterpriseLicense, k *rsa.PublicKey) {
				require.Equal(t, *l, estype.EnterpriseLicense{})
				require.Nil(t, k)
			},
			wantErr: true,
		},
		{
			name: "successful trial start",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&licenseFixture)),
				l: &licenseFixture,
			},
			want: func(l *estype.EnterpriseLicense, k *rsa.PublicKey) {
				require.NotNil(t, k)
				require.NoError(t, l.IsMissingFields())
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InitTrial(tt.args.c, tt.args.l)
			if (err != nil) != tt.wantErr {
				t.Errorf("InitTrial() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				tt.want(tt.args.l, got)
			}
		})
	}
}

func TestPopulateTrialLicense(t *testing.T) {
	type args struct {
		l *estype.EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(estype.EnterpriseLicense)
		wantErr    bool
	}{
		{
			name: "non-trial FAIL",
			args: args{
				l: &estype.EnterpriseLicense{
					Spec: estype.EnterpriseLicenseSpec{
						Type: estype.LicenseTypeEnterprise,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "trial license OK",
			args: args{
				l: &estype.EnterpriseLicense{
					ObjectMeta: v1.ObjectMeta{
						UID: "this-would-come-from-the-api-server",
					},
					Spec: estype.EnterpriseLicenseSpec{
						Type: estype.LicenseTypeEnterpriseTrial,
					},
				},
			},
			assertions: func(l estype.EnterpriseLicense) {
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
		l     *estype.EnterpriseLicense
	}
	tests := []struct {
		name       string
		args       args
		assertions func(estype.EnterpriseLicense)
	}{
		{
			name: "trial is 30 days",
			args: args{
				start: dateFixture,
				l:     &estype.EnterpriseLicense{},
			},
			assertions: func(license estype.EnterpriseLicense) {
				assert.Equal(t, license.ExpiryDate().UTC(), time.Date(2019, 02, 21, 0, 0, 0, 0, time.UTC))
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
