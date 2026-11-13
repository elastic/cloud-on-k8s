// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/chrono"
)

func TestLicenseVerifier_ValidSignature(t *testing.T) {
	rnd := rand.Reader
	privKey, err := rsa.GenerateKey(rnd, 2048)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		args        EnterpriseLicense
		verifyInput func(EnterpriseLicense) EnterpriseLicense
		wantErr     bool
	}{
		{
			name:    "valid v3 license",
			args:    licenseFixtureV3,
			wantErr: false,
		},
		{
			name:    "valid v4 license",
			args:    licenseFixtureV4,
			wantErr: false,
		},
		{
			name: "tampered v3 license",
			args: licenseFixtureV3,
			verifyInput: func(l EnterpriseLicense) EnterpriseLicense {
				l.License.MaxInstances = 1
				return l
			},
			wantErr: true,
		},
		{
			name: "tampered v4 license",
			args: licenseFixtureV4,
			verifyInput: func(l EnterpriseLicense) EnterpriseLicense {
				l.License.MaxResourceUnits = 1
				return l
			},
			wantErr: true,
		},
		{
			name: "empty signature",
			args: licenseFixtureV4,
			verifyInput: func(l EnterpriseLicense) EnterpriseLicense {
				l.License.Signature = ""
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
			toVerify := withSignature(tt.args, sig)
			if tt.verifyInput != nil {
				toVerify = tt.verifyInput(toVerify)
			}
			if err := v.ValidSignature(toVerify); (err != nil) != tt.wantErr {
				t.Errorf("Verifier.ValidSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckKeyFingerprint(t *testing.T) {
	verifierKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	verifierDER, err := x509.MarshalPKIXPublicKey(&verifierKey.PublicKey)
	require.NoError(t, err)
	otherDER, err := x509.MarshalPKIXPublicKey(&otherKey.PublicKey)
	require.NoError(t, err)

	matchingSHA256 := sha256.Sum256(verifierDER)
	mismatchSHA256 := sha256.Sum256(otherDER)

	tests := []struct {
		name        string
		fingerprint []byte
		wantErr     bool
		errContains string
	}{
		{
			name:        "SHA-256 match passes",
			fingerprint: matchingSHA256[:],
			wantErr:     false,
		},
		{
			name:        "SHA-256 mismatch returns wrong product error",
			fingerprint: mismatchSHA256[:],
			wantErr:     true,
			errContains: "different product",
		},
		{
			name:        "unparseable garbage falls through without error",
			fingerprint: []byte("not-a-valid-fingerprint"),
			wantErr:     false,
		},
		{
			name:        "unparseable random bytes fall through without error",
			fingerprint: make([]byte, 50),
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &Verifier{PublicKey: &verifierKey.PublicKey}
			err := verifier.checkKeyFingerprint(tt.fingerprint)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidSignature_ErrorMessages(t *testing.T) {
	signerKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	signer := NewSigner(signerKey)
	sig, err := signer.Sign(licenseFixtureV4)
	require.NoError(t, err)

	tests := []struct {
		name           string
		verifier       *Verifier
		license        EnterpriseLicense
		errContains    string
		errNotContains string
	}{
		{
			name:        "wrong key produces different product error",
			verifier:    &Verifier{PublicKey: &otherKey.PublicKey},
			license:     withSignature(licenseFixtureV4, sig),
			errContains: "different product",
		},
		{
			name:     "tampered content with correct key produces verification failed error",
			verifier: &Verifier{PublicKey: &signerKey.PublicKey},
			license: func() EnterpriseLicense {
				l := withSignature(licenseFixtureV4, sig)
				l.License.MaxResourceUnits = 9999
				return l
			}(),
			errContains:    "verification failed",
			errNotContains: "different product",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.verifier.ValidSignature(tt.license)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
			if tt.errNotContains != "" {
				assert.NotContains(t, err.Error(), tt.errNotContains)
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
				require.NoError(t, v.ValidSignature(licenseFixtureV3))
			},
		},
		{
			name: "Detects tampered license",
			want: func(v *Verifier) {
				l := licenseFixtureV3
				l.License.Issuer = "me"
				require.Error(t, v.ValidSignature(l))
			},
		},
		{
			name: "Detects empty signature",
			want: func(v *Verifier) {
				l := licenseFixtureV3
				l.License.Signature = ""
				require.Error(t, v.ValidSignature(l))
			},
		},
		{
			name: "Detects malicious signature",
			want: func(v *Verifier) {
				malice := make([]byte, base64.StdEncoding.DecodedLen(len(signatureFixtureV3)))
				_, err := base64.StdEncoding.Decode(malice, signatureFixtureV3)
				require.NoError(t, err)
				// inject max uint32 as the magic length
				malice[5] = 255
				malice[6] = 255
				malice[7] = 255
				malice[8] = 255
				tampered := make([]byte, base64.StdEncoding.EncodedLen(len(malice)))
				base64.StdEncoding.Encode(tampered, malice)
				err = v.ValidSignature(withSignature(licenseFixtureV3, tampered))
				require.Error(t, err)
				assert.Contains(t, err.Error(), "magic")
			},
		},
		{
			name: "Can recalculate signature",
			want: func(v *Verifier) {
				signer := NewSigner(privKey)
				bytes, err := signer.Sign(licenseFixtureV3)
				require.NoError(t, err)
				require.NoError(t, v.ValidSignature(withSignature(licenseFixtureV3, bytes)))
			},
		},
		{
			name: "Can verify license signed by external tooling",
			want: func(v *Verifier) {
				// license attributes contain <> and & which json.Marshal escapes by default leading to a signature
				// mismatch unless handled explicitly
				lic, err := externallySignedLicenseFixture()
				require.NoError(t, err)
				require.NoError(t, v.ValidSignature(lic))
			},
		},
		{
			name: "Can produce same signature as external tooling",
			want: func(v *Verifier) {
				signer := NewSigner(privKey)
				licenseSpec := EnterpriseLicense{
					License: LicenseSpec{
						UID:                "F983C1D2-1676-4427-8B6A-EF954AEEC174",
						Type:               "enterprise",
						IssueDateInMillis:  1606262400000,
						ExpiryDateInMillis: 1640995199999,
						MaxResourceUnits:   100,
						IssuedTo:           "ECK Unit & test <>",
						Issuer:             "ECK Unit tests",
						StartDateInMillis:  1606262400000,
						ClusterLicenses:    nil,
						Version:            4,
					},
				}
				sig, err := signer.Sign(licenseSpec)
				require.NoError(t, err)
				require.NoError(t, v.ValidSignature(withSignature(licenseSpec, sig)))

				lic, err := externallySignedLicenseFixture()
				require.NoError(t, err)

				expectedBytes, err := base64.StdEncoding.DecodeString(lic.License.Signature)
				require.NoError(t, err)
				actualBytes, err := base64.StdEncoding.DecodeString(string(sig))
				require.NoError(t, err)
				// some jiggery-pokery with knowledge of signature internals here to remove the random bits to allow a stable comparison
				require.Nil(t, deep.Equal(append(expectedBytes[:7], expectedBytes[21:]...), append(actualBytes[:7], actualBytes[21:]...)))
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
		l   EnterpriseLicense
		now time.Time
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   LicenseStatus
	}{
		{
			name: "valid license",
			fields: fields{
				PublicKey: publicKeyFixture(t),
			},
			args: args{
				l:   licenseFixtureV3,
				now: chrono.MustParseTime("2019-02-01"),
			},
			want: LicenseStatusValid,
		},
		{
			name: "expired license",
			fields: fields{
				PublicKey: publicKeyFixture(t),
			},
			args: args{
				l:   licenseFixtureV3,
				now: chrono.MustParseTime("2019-08-01"),
			},
			want: LicenseStatusExpired,
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
				l:   licenseFixtureV3,
				now: chrono.MustParseTime("2019-03-01"),
			},
			want: LicenseStatusInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &Verifier{
				PublicKey: tt.fields.PublicKey,
			}
			if got := v.Valid(context.Background(), tt.args.l, tt.args.now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Verifier.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestRealWorldLicenseFingerprint tests the fingerprint check against real license files.
// Set the following environment variables to run:
//
//	TEST_ORCHESTRATION_LICENSE_PATH - path to a JSON orchestration/ECK license file
//	TEST_STACK_LICENSE_PATH         - path to a JSON Elastic Stack license file
//	TEST_LICENSE_PUBKEY_PATH        - path to the DER-encoded public key for the orchestration license
//
// Note: in dev environments, both license types may share the same signing key. In that case,
// the Stack license will pass the fingerprint check but fail RSA content verification
// (the improved fallback message still applies). In production, different signing keys
// would trigger the "different product" fingerprint error.
func TestRealWorldLicenseFingerprint(t *testing.T) {
	orchPath := os.Getenv("TEST_ORCHESTRATION_LICENSE_PATH")
	stackPath := os.Getenv("TEST_STACK_LICENSE_PATH")
	pubKeyPath := os.Getenv("TEST_LICENSE_PUBKEY_PATH")
	if orchPath == "" || stackPath == "" || pubKeyPath == "" {
		t.Skip("set TEST_ORCHESTRATION_LICENSE_PATH, TEST_STACK_LICENSE_PATH, and TEST_LICENSE_PUBKEY_PATH to run this test")
	}

	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	require.NoError(t, err)
	verifier, err := NewVerifier(pubKeyBytes)
	require.NoError(t, err)

	t.Run("orchestration license passes signature check", func(t *testing.T) {
		lic := loadLicenseFromFile(t, orchPath)
		require.NoError(t, verifier.ValidSignature(lic))
	})

	t.Run("stack license fails signature check", func(t *testing.T) {
		lic := loadLicenseFromFile(t, stackPath)
		err := verifier.ValidSignature(lic)
		require.Error(t, err)
		// The error is either "different product" (production: different signing keys)
		// or "verification failed" (dev: same signing key, different content format).
		errMsg := err.Error()
		assert.True(t,
			strings.Contains(errMsg, "different product") || strings.Contains(errMsg, "verification failed"),
			"expected either a fingerprint mismatch or a signature verification failure, got: %v", err)
	})
}

func loadLicenseFromFile(t *testing.T, path string) EnterpriseLicense {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var lic EnterpriseLicense
	require.NoError(t, json.Unmarshal(data, &lic))
	return lic
}
