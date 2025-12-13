// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
)

func Test_newMetadataFor(t *testing.T) {
	tests := []struct {
		name         string
		policy       *autoopsv1alpha1.AutoOpsAgentPolicy
		es           *esv1.Elasticsearch
		expectedHash string
		want         map[string]string
	}{
		{
			name: "happy path",
			policy: &autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			expectedHash: "hash123",
			want: map[string]string{
				commonapikey.MetadataKeyConfigHash:  "hash123",
				commonapikey.MetadataKeyESName:      "es-1",
				commonapikey.MetadataKeyESNamespace: "ns-2",
				commonapikey.MetadataKeyManagedBy:   commonapikey.MetadataValueECK,
				PolicyNameLabelKey:                  "policy-1",
				policyNamespaceLabelKey:             "ns-1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newMetadataFor(tt.policy, tt.es, tt.expectedHash)
			if !cmp.Equal(got, tt.want) {
				t.Errorf("newMetadataFor() diff = %v", cmp.Diff(got, tt.want))
			}
		})
	}
}

func Test_buildAutoOpsESAPIKeySecret(t *testing.T) {
	tests := []struct {
		name         string
		policy       autoopsv1alpha1.AutoOpsAgentPolicy
		es           esv1.Elasticsearch
		secretName   string
		encodedKey   string
		expectedHash string
		want         corev1.Secret
	}{
		{
			name: "basic secret",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
					Labels: map[string]string{
						"label1": "value1",
					},
					Annotations: map[string]string{
						"annotation1": "value1",
					},
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			secretName:   "secret-1",
			encodedKey:   "encoded-api-key-value",
			expectedHash: "hash123",
			want: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-1",
					Namespace: "ns-1",
					Labels: map[string]string{
						commonapikey.MetadataKeyConfigHash:  "hash123",
						commonapikey.MetadataKeyESName:      "es-1",
						commonapikey.MetadataKeyESNamespace: "ns-2",
						PolicyNameLabelKey:                  "policy-1",
						policyNamespaceLabelKey:             "ns-1",
					},
					Annotations: map[string]string{
						"annotation1": "value1",
					},
				},
				Data: map[string][]byte{
					apiKeySecretKey: []byte("encoded-api-key-value"),
				},
			},
		},
		{
			name: "no labels or annotations on policy has no change in behavior",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			secretName:   "secret-1",
			encodedKey:   "key",
			expectedHash: "hash123",
			want: corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-1",
					Namespace: "ns-1",
					Labels: map[string]string{
						commonapikey.MetadataKeyConfigHash:  "hash123",
						commonapikey.MetadataKeyESName:      "es-1",
						commonapikey.MetadataKeyESNamespace: "ns-2",
						PolicyNameLabelKey:                  "policy-1",
						policyNamespaceLabelKey:             "ns-1",
					},
				},
				Data: map[string][]byte{
					apiKeySecretKey: []byte("key"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAutoOpsESAPIKeySecret(tt.policy, tt.es, tt.secretName, tt.encodedKey, tt.expectedHash)
			if !cmp.Equal(got, tt.want) {
				t.Errorf("buildAutoOpsESAPIKeySecret() diff = %v", cmp.Diff(got, tt.want))
			}
		})
	}
}
