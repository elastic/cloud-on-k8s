// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_newMetadataFor(t *testing.T) {
	tests := []struct {
		name         string
		policy       *autoopsv1alpha1.AutoOpsAgentPolicy
		es           *esv1.Elasticsearch
		expectedHash string
		want         map[string]any
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
			want: map[string]any{
				commonapikey.MetadataKeyConfigHash:  "hash123",
				commonapikey.MetadataKeyESName:      "es-1",
				commonapikey.MetadataKeyESNamespace: "ns-2",
				commonapikey.MetadataKeyManagedBy:   commonapikey.MetadataValueECK,
				commonv1.TypeLabelName:              autoOpsAgentType,
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
						commonapikey.MetadataKeyConfigHash:    "hash123",
						commonapikey.MetadataKeyESName:        "es-1",
						commonapikey.MetadataKeyESNamespace:   "ns-2",
						commonv1.TypeLabelName:                autoOpsAgentType,
						policySecretTypeLabelKey:              "api-key",
						PolicyNameLabelKey:                    "policy-1",
						policyNamespaceLabelKey:               "ns-1",
						commonv1.LabelBasedDiscoveryLabelName: commonv1.LabelBasedDiscoveryLabelValue,
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
						commonapikey.MetadataKeyConfigHash:    "hash123",
						commonapikey.MetadataKeyESName:        "es-1",
						commonapikey.MetadataKeyESNamespace:   "ns-2",
						commonv1.TypeLabelName:                autoOpsAgentType,
						policySecretTypeLabelKey:              "api-key",
						PolicyNameLabelKey:                    "policy-1",
						policyNamespaceLabelKey:               "ns-1",
						commonv1.LabelBasedDiscoveryLabelName: commonv1.LabelBasedDiscoveryLabelValue,
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

// Test_maybeUpdateAPIKey_AddsLabelBasedDiscoveryLabel verifies that when the API key
// is up to date and the secret already exists, the reconciler still updates the secret
// to add the label-based discovery label introduced for the watch mechanism.
func Test_maybeUpdateAPIKey_AddsLabelBasedDiscoveryLabel(t *testing.T) {
	scheme.SetupScheme()

	policy := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "ns-1",
		},
	}
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-1",
			Namespace: "ns-1",
			Labels:    map[string]string{"app": "elasticsearch"},
		},
	}

	const expectedHash = "hash123"
	apiKeyName := apiKeyNameFor(policy, es)
	secretName := autoopsv1alpha1.APIKeySecret(policy.GetName(), k8s.ExtractNamespacedName(&es))

	// Pre-existing secret without the LabelBasedDiscoveryLabelName label - this
	// simulates a secret created before the label was introduced.
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: policy.Namespace,
			Labels: map[string]string{
				PolicyNameLabelKey: policy.Name,
			},
		},
		Data: map[string][]byte{
			apiKeySecretKey: []byte("existing-encoded-api-key"),
		},
	}

	// Active API key that is managed by ECK and has a matching hash so that
	// maybeUpdateAPIKey enters the "key is up to date" branch.
	activeAPIKey := &esclient.APIKey{
		ID:   "api-key-id",
		Name: apiKeyName,
		Metadata: map[string]any{
			commonapikey.MetadataKeyManagedBy:  commonapikey.MetadataValueECK,
			commonapikey.MetadataKeyConfigHash: expectedHash,
		},
	}

	k8sClient := k8s.NewFakeClient(existingSecret)
	r := &AgentPolicyReconciler{
		Client:           k8sClient,
		esClientProvider: newFakeESClientProvider().Provider,
		params: operator.Parameters{
			Dialer: &fakeDialer{},
		},
		dynamicWatches: watches.NewDynamicWatches(),
	}

	got, err := r.maybeUpdateAPIKey(
		context.Background(),
		logr.Discard(),
		activeAPIKey,
		apiKeyName,
		apiKeySpec{},
		expectedHash,
		policy,
		es,
	)
	require.NoError(t, err)
	require.NotNil(t, got)

	// The returned secret must carry the new label.
	require.Equal(t,
		commonv1.LabelBasedDiscoveryLabelValue,
		got.Labels[commonv1.LabelBasedDiscoveryLabelName],
		"returned secret is missing the label-based discovery label",
	)

	// The persisted secret in the API server must also carry the new label.
	var persisted corev1.Secret
	require.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      secretName,
	}, &persisted))
	require.Equal(t,
		commonv1.LabelBasedDiscoveryLabelValue,
		persisted.Labels[commonv1.LabelBasedDiscoveryLabelName],
		"persisted secret is missing the label-based discovery label",
	)

	// The API key value must be preserved (not rotated).
	require.Equal(t, []byte("existing-encoded-api-key"), persisted.Data[apiKeySecretKey])
}
