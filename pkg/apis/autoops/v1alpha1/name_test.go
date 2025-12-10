// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func TestCASecret(t *testing.T) {
	maxLength := 27
	tests := []struct {
		name       string
		policyName string
		es         esv1.Elasticsearch
		want       string
	}{
		{
			name:       "test-cas-secret-too-long",
			policyName: "eck-autoops-config-policy",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testing",
					Namespace: "elastic",
				},
			},
			want: "eck-autoops-config-policy-autoops-ca-4269947480",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CASecret(tt.policyName, tt.es)
			if got != tt.want {
				t.Errorf("CASecret() = %v, want %v", got, tt.want)
			}
			suffix, _ := strings.CutPrefix(got, tt.policyName)
			if len(suffix) > maxLength {
				t.Errorf("CASecret(): suffix %s: length %d, want length %d", suffix, len(suffix), maxLength)
			}
		})
	}
}

func TestAPIKeySecret(t *testing.T) {
	maxLength := 27
	tests := []struct {
		name       string
		policyName string
		es         esv1.Elasticsearch
		want       string
	}{
		{
			name:       "test-api-key-secret-too-long",
			policyName: "eck-autoops-config-policy",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testing",
					Namespace: "elastic",
				},
			},
			want: "eck-autoops-config-policy-autoops-apikey-4269947480",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := APIKeySecret(tt.policyName, tt.es)
			if got != tt.want {
				t.Errorf("APIKeySecret() = %v, want %v", got, tt.want)
			}
			suffix, _ := strings.CutPrefix(got, tt.policyName)
			if len(suffix) > maxLength {
				t.Errorf("CASecret(): suffix %s: length %d, want length %d", suffix, len(suffix), maxLength)
			}
		})
	}
}
