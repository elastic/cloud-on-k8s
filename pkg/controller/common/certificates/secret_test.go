// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
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
