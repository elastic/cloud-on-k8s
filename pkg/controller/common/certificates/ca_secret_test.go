// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestParseCustomCASecret(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	key := loadFileBytes("tls.key")
	corruptedKey := loadFileBytes("corrupted.key")
	encryptedKey := loadFileBytes("encrypted.key")

	caFileName := "ca.crt"
	caKeyFileName := "ca.key"
	keyFileName := "tls.key"
	certFileName := "tls.crt"

	tests := []struct {
		name    string
		s       corev1.Secret
		wantErr bool
	}{
		{
			name: "Happy path",
			s: corev1.Secret{
				Data: map[string][]byte{
					caFileName:    ca,
					caKeyFileName: key,
				},
			},
			wantErr: false,
		},
		{
			name: "Happy path w/ legacy keys",
			s: corev1.Secret{
				Data: map[string][]byte{
					certFileName: ca,
					keyFileName:  key,
				},
			},
			wantErr: false,
		},
		{
			name: "Both ca and legacy keys",
			s: corev1.Secret{
				Data: map[string][]byte{
					certFileName:  ca,
					caKeyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "Both ca and legacy keys",
			s: corev1.Secret{
				Data: map[string][]byte{
					caFileName:  ca,
					keyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "Both ca and legacy keys",
			s: corev1.Secret{
				Data: map[string][]byte{
					certFileName:  ca,
					caFileName:    ca,
					keyFileName:   key,
					caKeyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "Empty certificate",
			s: corev1.Secret{
				Data: map[string][]byte{},
			},
			wantErr: true,
		},
		{
			name: "No cert",
			s: corev1.Secret{
				Data: map[string][]byte{
					caKeyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "No key",
			s: corev1.Secret{
				Data: map[string][]byte{
					caFileName: ca,
				},
			},
			wantErr: true,
		},
		{
			name: "Corrupted key",
			s: corev1.Secret{
				Data: map[string][]byte{
					caFileName:    ca,
					caKeyFileName: corruptedKey,
				},
			},
			wantErr: true,
		},
		{
			name: "Encrypted private key",
			s: corev1.Secret{
				Data: map[string][]byte{
					caFileName:    ca,
					caKeyFileName: encryptedKey,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCustomCASecret(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("ParseCustomCASecret() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCustomCASecretWithKeys(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	key := loadFileBytes("tls.key")

	tests := []struct {
		name            string
		s               corev1.Secret
		certKeyOverride string
		keyKeyOverride  string
		wantErr         bool
	}{
		{
			name: "Empty overrides → identical to ParseCustomCASecret (ca.* path)",
			s: corev1.Secret{
				Data: map[string][]byte{
					"ca.crt": ca,
					"ca.key": key,
				},
			},
			wantErr: false,
		},
		{
			name: "Both overrides set → reads tls.* even when ca.crt also present (cert-manager shape)",
			s: corev1.Secret{
				Data: map[string][]byte{
					"tls.crt": ca,
					"tls.key": key,
					"ca.crt":  ca, // cert-manager emits this as the issuing CA bundle
				},
			},
			certKeyOverride: "tls.crt",
			keyKeyOverride:  "tls.key",
			wantErr:         false,
		},
		{
			name: "Only cert override set → key falls back to ca.key default",
			s: corev1.Secret{
				Data: map[string][]byte{
					"tls.crt": ca,
					"ca.key":  key,
				},
			},
			certKeyOverride: "tls.crt",
			wantErr:         false,
		},
		{
			name: "Only key override set → cert falls back to ca.crt default",
			s: corev1.Secret{
				Data: map[string][]byte{
					"ca.crt":  ca,
					"tls.key": key,
				},
			},
			keyKeyOverride: "tls.key",
			wantErr:        false,
		},
		{
			name: "Override pointing at missing key returns error",
			s: corev1.Secret{
				Data: map[string][]byte{
					"ca.crt": ca,
					"ca.key": key,
				},
			},
			certKeyOverride: "does-not-exist.crt",
			keyKeyOverride:  "does-not-exist.key",
			wantErr:         true,
		},
		{
			name: "Override skips the both-exist conflict check",
			s: corev1.Secret{
				Data: map[string][]byte{
					"tls.crt": ca,
					"tls.key": key,
					"ca.crt":  ca,
					"ca.key":  key,
				},
			},
			certKeyOverride: "tls.crt",
			keyKeyOverride:  "tls.key",
			wantErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCustomCASecretWithKeys(tt.s, tt.certKeyOverride, tt.keyKeyOverride)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCustomCASecretWithKeys() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCustomCA(t *testing.T) {
	tests := []struct {
		name    string
		ca      func() *CA
		wantErr bool
	}{
		{
			name: "valid ca",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				return testCa
			},
			wantErr: false,
		},
		{
			name: "expired ca",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				testCa.Cert.NotAfter = time.Now().Add(-1 * time.Hour)
				return testCa
			},
			wantErr: true,
		},
		{
			name: "not valid yet ca",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				testCa.Cert.NotBefore = time.Now().Add(1 * time.Hour)
				return testCa
			},
			wantErr: true,
		},
		{
			name: "cert public key & private key mismatch",
			ca: func() *CA {
				testCa, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				testCa.PrivateKey = privateKey2
				return testCa
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCustomCA(t.Context(), tt.ca())
			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
			} else {
				require.NoError(t, err, "expected no err")
			}
		})
	}
}
