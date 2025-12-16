// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorejob

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type fakeESClient struct {
	esclient.Client
	reloadResponse esclient.ReloadSecureSettingsResponse
	reloadErr      error
}

func (f *fakeESClient) ReloadSecureSettings(ctx context.Context) (esclient.ReloadSecureSettingsResponse, error) {
	return f.reloadResponse, f.reloadErr
}

func TestReloadSecureSettings(t *testing.T) {
	const (
		esName      = "test-es"
		esNamespace = "test-ns"
		digest      = "abc123"
	)

	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esName,
			Namespace: esNamespace,
		},
	}

	tests := []struct {
		name              string
		keystoreSecret    *corev1.Secret
		esClient          *fakeESClient
		expectedNodeCount int32
		wantConverged     bool
		wantErr           bool
	}{
		{
			name: "all nodes converged",
			keystoreSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.KeystoreSecretName(esName),
					Namespace: esNamespace,
					Annotations: map[string]string{
						esv1.KeystoreDigestAnnotation: digest,
					},
				},
			},
			esClient: &fakeESClient{
				reloadResponse: esclient.ReloadSecureSettingsResponse{
					ClusterName: "test-cluster",
					Nodes: map[string]esclient.ReloadSecureSettingsNode{
						"node-1": {Name: "node-1", KeystoreDigest: digest},
						"node-2": {Name: "node-2", KeystoreDigest: digest},
					},
				},
			},
			expectedNodeCount: 2,
			wantConverged:     true,
		},
		{
			name: "some nodes have wrong digest",
			keystoreSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.KeystoreSecretName(esName),
					Namespace: esNamespace,
					Annotations: map[string]string{
						esv1.KeystoreDigestAnnotation: digest,
					},
				},
			},
			esClient: &fakeESClient{
				reloadResponse: esclient.ReloadSecureSettingsResponse{
					ClusterName: "test-cluster",
					Nodes: map[string]esclient.ReloadSecureSettingsNode{
						"node-1": {Name: "node-1", KeystoreDigest: digest},
						"node-2": {Name: "node-2", KeystoreDigest: "different-digest"},
					},
				},
			},
			expectedNodeCount: 2,
			wantConverged:     false,
		},
		{
			name: "not all expected nodes responded",
			keystoreSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.KeystoreSecretName(esName),
					Namespace: esNamespace,
					Annotations: map[string]string{
						esv1.KeystoreDigestAnnotation: digest,
					},
				},
			},
			esClient: &fakeESClient{
				reloadResponse: esclient.ReloadSecureSettingsResponse{
					ClusterName: "test-cluster",
					Nodes: map[string]esclient.ReloadSecureSettingsNode{
						"node-1": {Name: "node-1", KeystoreDigest: digest},
					},
				},
			},
			expectedNodeCount: 2, // expecting 2 but only 1 responded
			wantConverged:     false,
		},
		{
			name: "no digest annotation on secret",
			keystoreSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.KeystoreSecretName(esName),
					Namespace: esNamespace,
					// No annotations
				},
			},
			esClient:          &fakeESClient{},
			expectedNodeCount: 2,
			wantConverged:     false,
		},
		{
			name: "no nodes in response",
			keystoreSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.KeystoreSecretName(esName),
					Namespace: esNamespace,
					Annotations: map[string]string{
						esv1.KeystoreDigestAnnotation: digest,
					},
				},
			},
			esClient: &fakeESClient{
				reloadResponse: esclient.ReloadSecureSettingsResponse{
					ClusterName: "test-cluster",
					Nodes:       map[string]esclient.ReloadSecureSettingsNode{},
				},
			},
			expectedNodeCount: 2,
			wantConverged:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(tt.keystoreSecret)

			result, err := ReloadSecureSettings(context.Background(), client, tt.esClient, es, tt.expectedNodeCount)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantConverged, result.Converged)
		})
	}
}
