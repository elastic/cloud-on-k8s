// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_internalValidate(t *testing.T) {
	tests := []struct {
		name    string
		secret  corev1.Secret
		wantErr bool
	}{
		{
			name: "valid config with all required fields",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmAPIKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
					autoOpsToken:   []byte("token-value"),
				},
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
			wantErr: true,
		},
		{
			name: "missing autoOpsOTelURL returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmAPIKey:    []byte("ccm-api-key-value"),
					autoOpsToken: []byte("token-value"),
				},
			},
			wantErr: true,
		},
		{
			name: "missing autoOpsToken returns an error",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmAPIKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
				},
			},
			wantErr: true,
		},
		{
			name: "empty values are not allowed",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmAPIKey:      []byte(""),
					autoOpsOTelURL: []byte(""),
					autoOpsToken:   []byte(""),
				},
			},
			wantErr: true,
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
			wantErr: true,
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
			wantErr: true,
		},
		{
			name: "extra fields are ignored",
			secret: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					ccmAPIKey:      []byte("ccm-api-key-value"),
					autoOpsOTelURL: []byte("https://otel.example.com"),
					autoOpsToken:   []byte("token-value"),
					"extra-field":  []byte("extra-value"),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := internalValidate(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("internalValidate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
