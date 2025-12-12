// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	netutil "github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// fakeESClientProvider provides a fake ES client
type fakeESClientProvider struct {
	client *fakeESClient
}

func newFakeESClientProvider() *fakeESClientProvider {
	return &fakeESClientProvider{
		client: &fakeESClient{},
	}
}

func newFakeESClientProviderWithClient(client *fakeESClient) *fakeESClientProvider {
	return &fakeESClientProvider{
		client: client,
	}
}

func (f *fakeESClientProvider) Provider(ctx context.Context, c k8s.Client, dialer netutil.Dialer, es esv1.Elasticsearch) (esclient.Client, error) {
	return f.client, nil
}

type fakeESClient struct {
	esclient.Client
	getAPIKeysByNameErr error
}

func (f *fakeESClient) Close() {
}

func (f *fakeESClient) GetAPIKeysByName(ctx context.Context, name string) (esclient.APIKeyList, error) {
	if f.getAPIKeysByNameErr != nil {
		return esclient.APIKeyList{}, f.getAPIKeysByNameErr
	}
	return esclient.APIKeyList{APIKeys: []esclient.APIKey{}}, nil
}

func (f *fakeESClient) CreateAPIKey(ctx context.Context, req esclient.APIKeyCreateRequest) (esclient.APIKeyCreateResponse, error) {
	return esclient.APIKeyCreateResponse{
		ID:      req.ID,
		Name:    req.Name,
		APIKey:  req.APIKey.Name,
		Encoded: "dGVzdC1hcGkta2V5LWlkOnRlc3QtYXBpLWtleQ==",
	}, nil
}

func (f *fakeESClient) InvalidateAPIKeys(ctx context.Context, req esclient.APIKeysInvalidateRequest) (esclient.APIKeysInvalidateResponse, error) {
	return esclient.APIKeysInvalidateResponse{}, nil
}

type fakeDialer struct{}

func (f *fakeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return nil, nil
}

func TestReconcileAutoOpsAgentPolicy_onDelete(t *testing.T) {
	tests := []struct {
		name             string
		policy           types.NamespacedName
		secrets          []corev1.Secret
		esClusters       []esv1.Elasticsearch
		esClientProvider commonesclient.Provider
		wantErr          bool
	}{
		{
			name: "cleanup API keys for single ES cluster",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-key"),
					},
				},
			},
			esClusters: []esv1.Elasticsearch{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-2",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cleanup API keys for multiple ES clusters",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-key-1"),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-2-ns-3-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							"autoops.k8s.elastic.co/policy-name":      "policy-1",
							"autoops.k8s.elastic.co/policy-namespace": "ns-1",
							"elasticsearch.k8s.elastic.co/name":       "es-2",
							"elasticsearch.k8s.elastic.co/namespace":  "ns-3",
						},
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-key-2"),
					},
				},
			},
			esClusters: []esv1.Elasticsearch{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-2",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-2",
						Namespace: "ns-3",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "skip secrets without ES cluster labels",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secret-without-labels",
						Namespace: "ns-1",
						Labels: map[string]string{
							"autoops.k8s.elastic.co/policy-name":      "policy-1",
							"autoops.k8s.elastic.co/policy-namespace": "ns-1",
							// Missing ES cluster labels
						},
					},
				},
			},
			esClusters: []esv1.Elasticsearch{},
			wantErr:    false,
		},
		{
			name: "skip ES cluster that is not found",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
				},
			},
			esClusters: []esv1.Elasticsearch{},
			wantErr:    false,
		},
		{
			name: "deduplicate ES clusters from multiple secrets",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key-duplicate",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
				},
			},
			esClusters: []esv1.Elasticsearch{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-2",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "handle ES cluster error when getting API keys",
			policy: types.NamespacedName{
				Namespace: "ns-1",
				Name:      "policy-1",
			},
			secrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-ns-2-autoops-es-api-key",
						Namespace: "ns-1",
						Labels: map[string]string{
							PolicyNameLabelKey:                       "policy-1",
							policyNamespaceLabelKey:                  "ns-1",
							"elasticsearch.k8s.elastic.co/name":      "es-1",
							"elasticsearch.k8s.elastic.co/namespace": "ns-2",
						},
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-key"),
					},
				},
			},
			esClusters: []esv1.Elasticsearch{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-2",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			esClientProvider: newFakeESClientProviderWithClient(&fakeESClient{
				getAPIKeysByNameErr: errors.New("elasticsearch cluster unavailable"),
			}).Provider,
			wantErr: false, // onDelete continues on error, hence no error wanted here
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]client.Object, 0)
			for i := range tt.secrets {
				objects = append(objects, &tt.secrets[i])
			}
			for i := range tt.esClusters {
				objects = append(objects, &tt.esClusters[i])
			}

			k8sClient := k8s.NewFakeClient(objects...)
			esClientProvider := tt.esClientProvider
			if esClientProvider == nil {
				esClientProvider = newFakeESClientProvider().Provider
			}

			r := &AgentPolicyReconciler{
				Client:           k8sClient,
				esClientProvider: esClientProvider,
				params: operator.Parameters{
					Dialer: &fakeDialer{},
				},
				dynamicWatches: watches.NewDynamicWatches(),
			}

			ctx := context.Background()
			err := r.onDelete(ctx, tt.policy)

			require.Equal(t, tt.wantErr, err != nil)

			for _, es := range tt.esClusters {
				if es.Status.Phase == esv1.ElasticsearchReadyPhase {
					expectedSecretName := autoopsv1alpha1.APIKeySecret(tt.policy.Name, k8s.ExtractNamespacedName(&es))
					var retrievedSecret corev1.Secret
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: tt.policy.Namespace, Name: expectedSecretName}, &retrievedSecret)
					assert.True(t, apierrors.IsNotFound(err), "Expected secret %s/%s to be deleted", tt.policy.Namespace, expectedSecretName)
				}
			}
		})
	}
}
