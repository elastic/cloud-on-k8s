// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_validateAndPopulateConfig(t *testing.T) {
	tests := []struct {
		name      string
		secret    corev1.Secret
		secretKey types.NamespacedName
		want      *Config
		wantErr   bool
	}{
		{
			name: "valid config with all required fields",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmApiKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
					autoOpsToken:   []byte("token-value"),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want: &Config{
				CCMApiKey:      "ccm-api-key-value",
				AutoOpsOTelURL: "https://otel.example.com",
				AutoOpsToken:   "token-value",
			},
			wantErr: false,
		},
		{
			name: "missing ccmApiKey returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					autoOpsOTelURL: []byte("https://otel.example.com"),
					autoOpsToken:   []byte("token-value"),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "missing autoOpsOTelURL returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmApiKey:    []byte("ccm-api-key-value"),
					autoOpsToken: []byte("token-value"),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "missing autoOpsToken returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmApiKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "empty values are not allowed",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmApiKey:      []byte(""),
					autoOpsOTelURL: []byte(""),
					autoOpsToken:   []byte(""),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "empty secret data returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "nil secret data returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: nil,
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want:      nil,
			wantErr:   true,
		},
		{
			name: "extra fields are ignored",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmApiKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
					autoOpsToken:   []byte("token-value"),
					"extra-field":  []byte("extra-value"),
				},
			},
			secretKey: types.NamespacedName{Name: "config-secret", Namespace: "default"},
			want: &Config{
				CCMApiKey:      "ccm-api-key-value",
				AutoOpsOTelURL: "https://otel.example.com",
				AutoOpsToken:   "token-value",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndPopulateConfig(tt.secret, tt.secretKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAndPopulateConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got == nil {
					t.Errorf("validateAndPopulateConfig() got = nil, want %v", tt.want)
					return
				}
				if got.CCMApiKey != tt.want.CCMApiKey {
					t.Errorf("validateAndPopulateConfig() ccmApiKey = %v, want %v", got.CCMApiKey, tt.want.CCMApiKey)
				}
				if got.AutoOpsOTelURL != tt.want.AutoOpsOTelURL {
					t.Errorf("validateAndPopulateConfig() autoOpsOTelURL = %v, want %v", got.AutoOpsOTelURL, tt.want.AutoOpsOTelURL)
				}
				if got.AutoOpsToken != tt.want.AutoOpsToken {
					t.Errorf("validateAndPopulateConfig() autoOpsToken = %v, want %v", got.AutoOpsToken, tt.want.AutoOpsToken)
				}
			}
		})
	}
}
