// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const testNS = "test-system"

func TestChecker_EnterpriseFeaturesEnabled(t *testing.T) {
	ctx := context.Background()
	privKey, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)

	validLicenseFixture := licenseFixtureV3
	validLicenseFixture.License.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(1 * time.Hour))

	signatureBytes, err := NewSigner(privKey).Sign(validLicenseFixture)
	require.NoError(t, err)

	trialState, err := NewTrialState()
	require.NoError(t, err)
	validTrialLicenseFixture := emptyTrialLicenseFixture
	require.NoError(t, trialState.InitTrialLicense(ctx, &validTrialLicenseFixture))

	validLegacyTrialFixture := EnterpriseLicense{
		License: LicenseSpec{
			Type: LicenseTypeLegacyTrial,
		},
	}
	require.NoError(t, trialState.InitTrialLicense(ctx, &validLegacyTrialFixture))

	expiredTrialLicense := validTrialLicenseFixture
	expiredTrialLicense.License.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(-1 * time.Hour))
	expiredTrialSignatureBytes, err := NewSigner(trialState.privateKey).Sign(expiredTrialLicense)
	require.NoError(t, err)

	statusSecret, err := ExpectedTrialStatus(testNS, types.NamespacedName{}, trialState)
	require.NoError(t, err)

	type fields struct {
		initialObjects []client.Object
		publicKey      []byte
	}
	tests := []struct {
		name    string
		fields  fields
		want    bool
		wantErr bool
	}{
		{
			name: "valid license: OK",
			fields: fields{
				initialObjects: asClientObjects(validLicenseFixture, signatureBytes),
				publicKey:      publicKeyBytesFixture(t),
			},
			want: true,
		},
		{
			name: "valid trial: OK",
			fields: fields{
				initialObjects: []client.Object{asClientObject(validTrialLicenseFixture), &statusSecret},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "valid legacy trial: OK",
			fields: fields{
				initialObjects: []client.Object{asClientObject(validTrialLicenseFixture), &statusSecret},
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "invalid trial: FAIL",
			fields: fields{
				initialObjects: []client.Object{asClientObject(emptyTrialLicenseFixture), &statusSecret},
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "expired trial: FAIL",
			fields: fields{
				initialObjects: append(asClientObjects(expiredTrialLicense, expiredTrialSignatureBytes), &statusSecret),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "invalid signature: FAIL",
			fields: fields{
				initialObjects: asClientObjects(validLicenseFixture, []byte{}),
				publicKey:      publicKeyBytesFixture(t),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "no public key: FAIL",
			fields: fields{
				initialObjects: asClientObjects(validLicenseFixture, signatureBytes),
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &checker{
				k8sClient:         k8s.NewFakeClient(tt.fields.initialObjects...),
				operatorNamespace: testNS,
				publicKey:         tt.fields.publicKey,
			}
			got, err := lc.EnterpriseFeaturesEnabled(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Checker.EnterpriseFeaturesEnabled() err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Checker.EnterpriseFeaturesEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_CurrentEnterpriseLicense(t *testing.T) {
	privKey, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)

	validLicenseFixture := licenseFixtureV3
	validLicenseFixture.License.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(1 * time.Hour))
	signatureBytes, err := NewSigner(privKey).Sign(validLicenseFixture)
	require.NoError(t, err)
	validLicense := asClientObjects(validLicenseFixture, signatureBytes)

	trialState, err := NewTrialState()
	require.NoError(t, err)
	validTrialLicenseFixture := emptyTrialLicenseFixture
	require.NoError(t, trialState.InitTrialLicense(context.Background(), &validTrialLicenseFixture))
	validTrialLicense := asClientObject(validTrialLicenseFixture)

	statusSecret, err := ExpectedTrialStatus(testNS, types.NamespacedName{}, trialState)
	require.NoError(t, err)

	type fields struct {
		initialObjects    []client.Object
		operatorNamespace string
		publicKey         []byte
	}

	tests := []struct {
		name     string
		fields   fields
		want     bool
		wantErr  bool
		wantType OperatorLicenseType
	}{
		{
			name: "get valid enterprise license: OK",
			fields: fields{
				initialObjects:    validLicense,
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:     true,
			wantType: LicenseTypeEnterprise,
			wantErr:  false,
		},
		{
			name: "get valid trial enterprise license: OK",
			fields: fields{
				initialObjects:    []client.Object{validTrialLicense, &statusSecret},
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:     true,
			wantType: LicenseTypeEnterpriseTrial,
			wantErr:  false,
		},
		{
			name: "get valid enterprise license among two licenses: OK",
			fields: fields{
				initialObjects:    append(validLicense, validTrialLicense),
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:     true,
			wantType: LicenseTypeEnterprise,
			wantErr:  false,
		},
		{
			name: "no license: OK",
			fields: fields{
				initialObjects:    []client.Object{},
				operatorNamespace: "test-system",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "invalid public key: FAIL",
			fields: fields{
				initialObjects:    validLicense,
				operatorNamespace: "test-system",
				publicKey:         []byte("not a public key"),
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &checker{
				k8sClient:         k8s.NewFakeClient(tt.fields.initialObjects...),
				operatorNamespace: tt.fields.operatorNamespace,
				publicKey:         tt.fields.publicKey,
			}
			got, err := lc.CurrentEnterpriseLicense(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Checker.CurrentEnterpriseLicense() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.want != (got != nil) {
				t.Errorf("Checker.CurrentEnterpriseLicense() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_ValidOperatorLicenseKey(t *testing.T) {
	privKey, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)

	validLicenseFixture := licenseFixtureV3
	validLicenseFixture.License.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(1 * time.Hour))
	signatureBytes, err := NewSigner(privKey).Sign(validLicenseFixture)
	require.NoError(t, err)
	validLicense := asClientObjects(validLicenseFixture, signatureBytes)

	trialState, err := NewTrialState()
	require.NoError(t, err)
	validTrialLicenseFixture := emptyTrialLicenseFixture
	require.NoError(t, trialState.InitTrialLicense(context.Background(), &validTrialLicenseFixture))
	validTrialLicense := asClientObject(validTrialLicenseFixture)

	statusSecret, err := ExpectedTrialStatus(testNS, types.NamespacedName{}, trialState)
	require.NoError(t, err)

	type fields struct {
		initialObjects    []client.Object
		operatorNamespace string
		publicKey         []byte
	}

	tests := []struct {
		name     string
		fields   fields
		wantErr  bool
		wantType OperatorLicenseType
	}{
		{
			name: "get valid basic license: OK",
			fields: fields{
				initialObjects:    []client.Object{},
				operatorNamespace: "test-system",
			},
			wantType: LicenseTypeBasic,
			wantErr:  false,
		},
		{
			name: "get valid enterprise license: OK",
			fields: fields{
				initialObjects:    validLicense,
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			wantType: LicenseTypeEnterprise,
			wantErr:  false,
		},
		{
			name: "get valid trial enterprise license: OK",
			fields: fields{
				initialObjects:    []client.Object{validTrialLicense, &statusSecret},
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			wantType: LicenseTypeEnterpriseTrial,
			wantErr:  false,
		},
		{
			name: "get valid enterprise license among two licenses: OK",
			fields: fields{
				initialObjects:    append(validLicense, validTrialLicense),
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			wantType: LicenseTypeEnterprise,
			wantErr:  false,
		},
		{
			name: "invalid public key: fallback to basic",
			fields: fields{
				initialObjects:    validLicense,
				operatorNamespace: "test-system",
				publicKey:         []byte("not a public key"),
			},
			wantType: LicenseTypeBasic,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &checker{
				k8sClient:         k8s.NewFakeClient(tt.fields.initialObjects...),
				operatorNamespace: tt.fields.operatorNamespace,
				publicKey:         tt.fields.publicKey,
			}
			licenseType, err := lc.ValidOperatorLicenseKeyType(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Checker.ValidOperatorLicenseKeyType() err = %v, wantErr %v", err, tt.wantErr)
			}
			if licenseType != tt.wantType {
				t.Errorf("Checker.ValidOperatorLicenseKeyType() licenseType = %v, wantType %v", licenseType, tt.wantType)
			}
		})
	}
}
