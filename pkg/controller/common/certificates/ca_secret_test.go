// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"testing"

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
