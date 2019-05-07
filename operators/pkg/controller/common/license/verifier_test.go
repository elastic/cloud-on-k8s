// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"reflect"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLicenseVerifier_ValidSignature(t *testing.T) {
	rnd := rand.Reader
	privKey, err := rsa.GenerateKey(rnd, 2048)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		args        v1alpha1.EnterpriseLicense
		verifyInput func(v1alpha1.EnterpriseLicense) v1alpha1.EnterpriseLicense
		wantErr     bool
	}{
		{
			name:    "valid license",
			args:    licenseFixture,
			wantErr: false,
		},
		{
			name: "tampered license",
			args: licenseFixture,
			verifyInput: func(l v1alpha1.EnterpriseLicense) v1alpha1.EnterpriseLicense {
				l.Spec.MaxInstances = 1
				return l
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewSigner(privKey)
			sig, err := v.Sign(tt.args)
			require.NoError(t, err)
			toVerify := tt.args
			if tt.verifyInput != nil {
				toVerify = tt.verifyInput(tt.args)
			}
			if err := v.ValidSignature(toVerify, sig); (err != nil) != tt.wantErr {
				t.Errorf("Verifier.ValidSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewLicenseVerifier(t *testing.T) {

	privKey, err := x509.ParsePKCS1PrivateKey(privateKeyFixture)
	require.NoError(t, err)

	publicKey := privKey.PublicKey
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&publicKey)
	assert.NoError(t, err)

	tests := []struct {
		name string
		want func(verifier *Verifier)
	}{
		{
			name: "Can create verifier from pub key bytes",
			want: func(v *Verifier) {
				require.NoError(t, v.ValidSignature(licenseFixture, signatureFixture))
			},
		},
		{
			name: "Detects tampered license",
			want: func(v *Verifier) {
				l := licenseFixture
				l.Spec.Issuer = "me"
				require.Error(t, v.ValidSignature(l, signatureFixture))
			},
		},
		{
			name: "Detects malicious signature",
			want: func(v *Verifier) {
				malice := make([]byte, base64.StdEncoding.DecodedLen(len(signatureFixture)))
				_, err := base64.StdEncoding.Decode(malice, signatureFixture)
				require.NoError(t, err)
				// inject max uint32 as the magic length
				malice[5] = 255
				malice[6] = 255
				malice[7] = 255
				malice[8] = 255
				tampered := make([]byte, base64.StdEncoding.EncodedLen(len(malice)))
				base64.StdEncoding.Encode(tampered, malice)
				err = v.ValidSignature(licenseFixture, tampered)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "magic")
			},
		},
		{
			name: "Can recalculate signature",
			want: func(v *Verifier) {
				signer := NewSigner(privKey)
				bytes, err := signer.Sign(licenseFixture)
				require.NoError(t, err)
				require.NoError(t, v.ValidSignature(licenseFixture, bytes))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewVerifier(pubKeyBytes)
			if err != nil {
				t.Errorf("NewVerifier() error = %v", err)
				return
			}
			if tt.want != nil {
				tt.want(got)
			}
		})
	}
}

func TestVerifier_Valid(t *testing.T) {
	type fields struct {
		PublicKey *rsa.PublicKey
	}
	type args struct {
		l   v1alpha1.EnterpriseLicense
		sig []byte
		now time.Time
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   v1alpha1.LicenseStatus
	}{
		{
			name: "valid license",
			fields: fields{
				PublicKey: publicKeyFixture(t),
			},
			args: args{
				l:   licenseFixture,
				sig: signatureFixture,
				now: chrono.MustParseTime("2019-02-01"),
			},
			want: v1alpha1.LicenseStatusValid,
		},
		{
			name: "expired license",
			fields: fields{
				PublicKey: publicKeyFixture(t),
			},
			args: args{
				l:   licenseFixture,
				sig: signatureFixture,
				now: chrono.MustParseTime("2019-08-01"),
			},
			want: v1alpha1.LicenseStatusExpired,
		},
		{
			name: "invalid signature",
			fields: fields{
				PublicKey: func() *rsa.PublicKey {
					priv, err := rsa.GenerateKey(rand.Reader, 2048)
					require.NoError(t, err)
					return &priv.PublicKey
				}(),
			},
			args: args{
				l:   licenseFixture,
				sig: signatureFixture,
				now: chrono.MustParseTime("2019-03-01"),
			},
			want: v1alpha1.LicenseStatusInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Verifier{
				PublicKey: tt.fields.PublicKey,
			}
			if got := v.Valid(tt.args.l, tt.args.sig, tt.args.now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Verifier.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}
