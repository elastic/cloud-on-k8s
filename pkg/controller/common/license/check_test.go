// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestChecker_EnterpriseFeaturesEnabled(t *testing.T) {
	privKey, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)

	validLicenseFixture := licenseFixtureV3
	validLicenseFixture.License.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(1 * time.Hour))

	signatureBytes, err := NewSigner(privKey).Sign(validLicenseFixture)
	require.NoError(t, err)

	type fields struct {
		initialObjects    []runtime.Object
		operatorNamespace string
		publicKey         []byte
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
				initialObjects:    asRuntimeObjects(validLicenseFixture, signatureBytes),
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want: true,
		},
		{
			name: "invalid signature: FAIL",
			fields: fields{
				initialObjects:    asRuntimeObjects(validLicenseFixture, []byte{}),
				operatorNamespace: "test-system",
				publicKey:         publicKeyBytesFixture(t),
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "no public key: FAIL",
			fields: fields{
				initialObjects:    asRuntimeObjects(validLicenseFixture, signatureBytes),
				operatorNamespace: "test-system",
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lc := &checker{
				k8sClient:         k8s.WrappedFakeClient(tt.fields.initialObjects...),
				operatorNamespace: tt.fields.operatorNamespace,
				publicKey:         tt.fields.publicKey,
			}
			got, err := lc.EnterpriseFeaturesEnabled()
			if (err != nil) != tt.wantErr {
				t.Errorf("Checker.EnterpriseFeaturesEnabled() err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Checker.EnterpriseFeaturesEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
