// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSecret_Parse(t *testing.T) {
	ca := loadFileBytes("ca.crt")
	key := loadFileBytes("tls.key")
	corruptedKey := loadFileBytes("corrupted.key")
	encryptedKey := loadFileBytes("encrypted.key")

	tests := []struct {
		name    string
		s       corev1.Secret
		wantErr bool
	}{
		{
			name: "Happy path",
			s: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: ca,
					KeyFileName:  key,
				},
			},
			wantErr: false,
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
					KeyFileName: key,
				},
			},
			wantErr: true,
		},
		{
			name: "No key",
			s: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: ca,
				},
			},
			wantErr: true,
		},
		{
			name: "Corrupted key",
			s: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: ca,
					KeyFileName:  corruptedKey,
				},
			},
			wantErr: true,
		},
		{
			name: "Encrypted private key",
			s: corev1.Secret{
				Data: map[string][]byte{
					CertFileName: ca,
					KeyFileName:  encryptedKey,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseCustomCASecret(tt.s); (err != nil) != tt.wantErr {
				t.Errorf("Secret.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
