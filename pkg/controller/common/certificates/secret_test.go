// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestCertificatesSecret(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")
	chain := loadFileBytes("chain.crt")

	tests := []struct {
		name                                 string
		s                                    CertificatesSecret
		wantCa, wantCert, wantChain, wantKey []byte
		wantCAWithPrivateKey                 bool
	}{
		{
			name: "Simple chain",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:   ca,
						CertFileName: tls,
						KeyFileName:  key,
					},
				},
			},
			wantCa:               ca,
			wantKey:              key,
			wantCert:             tls,
			wantChain:            chain,
			wantCAWithPrivateKey: false,
		},
		{
			name: "No CA cert",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CertFileName: tls,
						KeyFileName:  key,
					},
				},
			},
			wantCa:               nil,
			wantKey:              key,
			wantCert:             tls,
			wantChain:            tls,
			wantCAWithPrivateKey: false,
		},
		{
			name: "Full CA cert",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:    ca,
						CAKeyFileName: []byte(testPemPrivateKey),
					},
				},
			},
			wantCa:               ca,
			wantCert:             nil,
			wantChain:            ca,
			wantKey:              nil,
			wantCAWithPrivateKey: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.CertChain(); !reflect.DeepEqual(got, tt.wantChain) {
				t.Errorf("CertificatesSecret.CertChain() = %v, want %v", got, tt.wantChain)
			}
			if got := tt.s.CAPem(); !reflect.DeepEqual(got, tt.wantCa) {
				t.Errorf("CertificatesSecret.CAPem() = %v, want %v", got, tt.wantCa)
			}
			if got := tt.s.CertPem(); !reflect.DeepEqual(got, tt.wantCert) {
				t.Errorf("CertificatesSecret.CertPem() = %v, want %v", got, tt.wantCert)
			}
			if got := tt.s.KeyPem(); !reflect.DeepEqual(got, tt.wantKey) {
				t.Errorf("CertificatesSecret.CertChain() = %v, want %v", got, tt.wantKey)
			}

			if tt.s.HasCAPrivateKey() != tt.wantCAWithPrivateKey {
				t.Errorf("CertificatesSecret.HasCAPrivateKey() = %v, want %v", tt.s.HasCAPrivateKey(), tt.wantCAWithPrivateKey)
			}
		})
	}
}

func TestCertificatesSecret_Validate(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")
	chain := loadFileBytes("chain.crt")
	corruptedKey := loadFileBytes("corrupted.key")
	encryptedKey := loadFileBytes("encrypted.key")

	tests := []struct {
		name    string
		s       CertificatesSecret
		wantErr bool
	}{
		{
			name: "Happy path",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:   ca,
						CertFileName: tls,
						KeyFileName:  key,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Empty certificate",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{},
				},
			},
			wantErr: true,
		},
		{
			name: "No cert",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						KeyFileName: key,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "No key",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:   ca,
						CertFileName: tls,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "No CA cert",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CertFileName: tls,
						KeyFileName:  key,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Encrypted key",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CertFileName: tls,
						KeyFileName:  encryptedKey,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Corrupted private key",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CertFileName: tls,
						KeyFileName:  corruptedKey,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "With CA private key",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:    ca,
						CAKeyFileName: []byte(testPemPrivateKey),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Mixed leaf and custom CA 1/2",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:    ca,
						CAKeyFileName: []byte(testPemPrivateKey),
						CertFileName:  tls,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Mixed leaf and custom CA 2/2",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:    ca,
						CAKeyFileName: []byte(testPemPrivateKey),
						KeyFileName:   key,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple CA certs",
			s: CertificatesSecret{
				Secret: v1.Secret{
					Data: map[string][]byte{
						CAFileName:    chain,
						CAKeyFileName: []byte(testPemPrivateKey),
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.s.parse(); (err != nil) != tt.wantErr {
				t.Errorf("CertificatesSecret.parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func loadFileBytes(fileName string) []byte {
	contents, err := os.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}

func Test_validCustomCertificatesOrNil_WithCustomCA(t *testing.T) {
	owner := types.NamespacedName{Namespace: "ns", Name: "es"}

	// Helper function to create a secret with custom CA
	createCustomCASecret := func(t *testing.T, ca *CA, secretName string) *v1.Secret {
		t.Helper()
		pemKey, err := EncodePEMPrivateKey(ca.PrivateKey)
		require.NoError(t, err)
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: owner.Namespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				CAFileName:    EncodePEMCert(ca.Cert.Raw),
				CAKeyFileName: pemKey,
			},
		}
	}

	tests := []struct {
		name     string
		customCA func(t *testing.T) *CA
		wantErr  bool
	}{
		{
			name: "valid custom CA should pass validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				testCA, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				return testCA
			},
			wantErr: false,
		},
		{
			name: "expired custom CA should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA that expired 1 hour ago
				expiredTime := -1 * time.Hour
				testCA, err := NewSelfSignedCA(CABuilderOptions{
					ExpireIn: &expiredTime,
				})
				require.NoError(t, err)
				return testCA
			},
			wantErr: true,
		},
		{
			name: "not-yet-valid custom CA should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA manually with NotBefore in the future
				privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				serial, err := cryptorand.Int(cryptorand.Reader, SerialNumberLimit)
				require.NoError(t, err)

				certificateTemplate := x509.Certificate{
					SerialNumber:          serial,
					Subject:               pkix.Name{CommonName: "test-ca"},
					NotBefore:             time.Now().Add(1 * time.Hour), // Not yet valid
					NotAfter:              time.Now().Add(2 * time.Hour),
					SignatureAlgorithm:    x509.SHA256WithRSA,
					IsCA:                  true,
					BasicConstraintsValid: true,
					KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
					ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
				}

				certData, err := x509.CreateCertificate(cryptorand.Reader, &certificateTemplate, &certificateTemplate, privateKey.Public(), privateKey)
				require.NoError(t, err)
				cert, err := x509.ParseCertificate(certData)
				require.NoError(t, err)

				return NewCA(privateKey, cert)
			},
			wantErr: true,
		},
		{
			name: "custom CA with mismatched keys should fail validation",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				testCA, err := NewSelfSignedCA(CABuilderOptions{})
				require.NoError(t, err)
				// Generate a different private key
				privateKey2, err := rsa.GenerateKey(cryptorand.Reader, 2048)
				require.NoError(t, err)
				testCA.PrivateKey = privateKey2
				return testCA
			},
			wantErr: true,
		},
		{
			name: "custom CA expiring soon should log warning but succeed",
			customCA: func(t *testing.T) *CA {
				t.Helper()
				// Create a CA that expires soon (within DefaultRotateBefore)
				shortValidity := DefaultRotateBefore / 2
				testCA, err := NewSelfSignedCA(CABuilderOptions{
					ExpireIn: &shortValidity,
				})
				require.NoError(t, err)
				return testCA
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customCA := tt.customCA(t)
			customCASecretName := "custom-ca-secret"

			// Create the custom CA secret
			customCASecret := createCustomCASecret(t, customCA, customCASecretName)
			c := k8s.NewFakeClient(customCASecret)

			tlsOptions := commonv1.TLSOptions{
				Certificate: commonv1.SecretRef{
					SecretName: customCASecretName,
				},
			}

			certSecret, err := validCustomCertificatesOrNil(context.Background(), c, owner, tlsOptions)

			if tt.wantErr {
				assert.Error(t, err, "expected error from validCustomCertificatesOrNil")
				assert.Nil(t, certSecret, "certSecret should be nil on error")
			} else {
				assert.NoError(t, err, "expected no error from validCustomCertificatesOrNil")
				assert.NotNil(t, certSecret, "certSecret should not be nil on success")
				assert.True(t, certSecret.HasCAPrivateKey(), "certSecret should have CA private key")
			}
		})
	}
}
