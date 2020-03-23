// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"
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
	}{
		{
			name: "Simple chain",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CAFileName:   ca,
					CertFileName: tls,
					KeyFileName:  key,
				},
			},
			wantCa:    ca,
			wantKey:   key,
			wantCert:  tls,
			wantChain: chain,
		},
		{
			name: "No CA cert",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CertFileName: tls,
					KeyFileName:  key,
				},
			},
			wantCa:    nil,
			wantKey:   key,
			wantCert:  tls,
			wantChain: tls,
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
		})
	}
}

func TestCertificatesSecret_Validate(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	tls := loadFileBytes("tls.crt")
	key := loadFileBytes("tls.key")
	corruptedKey := loadFileBytes("corrupted.key")

	tests := []struct {
		name    string
		s       CertificatesSecret
		wantErr bool
	}{
		{
			name: "Happy path",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CAFileName:   ca,
					CertFileName: tls,
					KeyFileName:  key,
				},
			},
			wantErr: false,
		},
		{
			name: "Empty certificate",
			s: CertificatesSecret{
				Data: map[string][]byte{},
			},
			wantErr: true,
		},
		{
			name: "No cert",
			s: CertificatesSecret{
				Data: map[string][]byte{
					KeyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "No key",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CAFileName:   ca,
					CertFileName: tls,
				},
			},
			wantErr: true,
		},
		{
			name: "No CA cert",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CertFileName: tls,
					KeyFileName:  key,
				},
			},
			wantErr: false,
		},
		{
			name: "Corrupted key",
			s: CertificatesSecret{
				Data: map[string][]byte{
					CertFileName: tls,
					KeyFileName:  corruptedKey,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.s.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("CertificatesSecret.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func loadFileBytes(fileName string) []byte {
	contents, err := ioutil.ReadFile(filepath.Join("testdata", fileName))
	if err != nil {
		panic(err)
	}

	return contents
}
