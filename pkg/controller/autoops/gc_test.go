// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestGarbageCollector_DoGarbageCollection(t *testing.T) {
	scheme.SetupScheme()

	// Helper to create a secret with policy labels
	createSecret := func(name, namespace, policyName, policyNamespace, esName, esNamespace, secretType string) *corev1.Secret {
		labels := map[string]string{
			PolicyNameLabelKey:                  policyName,
			policyNamespaceLabelKey:             policyNamespace,
			commonapikey.MetadataKeyESName:      esName,
			commonapikey.MetadataKeyESNamespace: esNamespace,
			commonv1.TypeLabelName:              autoOpsAgentType,
		}
		if secretType != "" {
			labels[policySecretTypeLabelKey] = secretType
		}
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels:    labels,
			},
		}
	}

	// Helper to create a configmap with policy labels
	createConfigMap := func(name, namespace, policyName, policyNamespace, esName, esNamespace string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					PolicyNameLabelKey:                  policyName,
					policyNamespaceLabelKey:             policyNamespace,
					commonapikey.MetadataKeyESName:      esName,
					commonapikey.MetadataKeyESNamespace: esNamespace,
					commonv1.TypeLabelName:              autoOpsAgentType,
				},
			},
		}
	}

	// Helper to create a deployment with policy labels
	createDeployment := func(name, namespace, policyName, policyNamespace, esName, esNamespace string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					PolicyNameLabelKey:                  policyName,
					policyNamespaceLabelKey:             policyNamespace,
					commonapikey.MetadataKeyESName:      esName,
					commonapikey.MetadataKeyESNamespace: esNamespace,
					commonv1.TypeLabelName:              autoOpsAgentType,
				},
			},
		}
	}

	// Helper to create an AutoOpsAgentPolicy
	createPolicy := func(name, namespace string) *autoopsv1alpha1.AutoOpsAgentPolicy {
		return &autoopsv1alpha1.AutoOpsAgentPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
				Version: "9.1.0",
			},
		}
	}

	// Helper to create an ES cluster
	createES := func(name, namespace string) *esv1.Elasticsearch {
		return &esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: esv1.ElasticsearchStatus{
				Phase: esv1.ElasticsearchReadyPhase,
			},
		}
	}

	tests := []struct {
		name            string
		objects         []client.Object
		wantSecrets     int
		wantConfigMaps  int
		wantDeployments int
	}{
		{
			name: "no orphaned resources when policy exists",
			objects: []client.Object{
				createPolicy("policy-1", "ns-1"),
				createES("es-1", "ns-1"),
				createSecret("secret-1", "ns-1", "policy-1", "ns-1", "es-1", "ns-1", apiKeySecretType),
				createConfigMap("configmap-1", "ns-1", "policy-1", "ns-1", "es-1", "ns-1"),
				createDeployment("deployment-1", "ns-1", "policy-1", "ns-1", "es-1", "ns-1"),
			},
			wantSecrets:     1,
			wantConfigMaps:  1,
			wantDeployments: 1,
		},
		{
			name: "cleanup orphaned secrets when policy is deleted",
			objects: []client.Object{
				// No policy exists, but resources reference it
				createES("es-1", "ns-1"),
				createSecret("secret-1", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1", apiKeySecretType),
				// ConfigMaps and Deployments have owner references, so Kubernetes GC handles them.
				// We still create them here to verify the GC doesn't error on their presence.
				createConfigMap("configmap-1", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1"),
				createDeployment("deployment-1", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1"),
			},
			wantSecrets:     0,
			wantConfigMaps:  1, // not cleaned up by GC, has owner reference
			wantDeployments: 1, // not cleaned up by GC, has owner reference
		},
		{
			name: "cleanup only orphaned secrets, keep secrets for existing policy",
			objects: []client.Object{
				createPolicy("existing-policy", "ns-1"),
				createES("es-1", "ns-1"),
				// Resources for existing policy
				createSecret("secret-existing", "ns-1", "existing-policy", "ns-1", "es-1", "ns-1", apiKeySecretType),
				createConfigMap("configmap-existing", "ns-1", "existing-policy", "ns-1", "es-1", "ns-1"),
				createDeployment("deployment-existing", "ns-1", "existing-policy", "ns-1", "es-1", "ns-1"),
				// Orphaned resources for deleted policy
				createSecret("secret-orphaned", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1", apiKeySecretType),
				createConfigMap("configmap-orphaned", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1"),
				createDeployment("deployment-orphaned", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1"),
			},
			wantSecrets:     1, // orphaned secret cleaned up
			wantConfigMaps:  2, // not cleaned up by GC, have owner references
			wantDeployments: 2, // not cleaned up by GC, have owner references
		},
		{
			name: "cleanup CA secrets as well as API key secrets",
			objects: []client.Object{
				createES("es-1", "ns-1"),
				createSecret("api-key-secret", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1", apiKeySecretType),
				createSecret("ca-secret", "ns-1", "deleted-policy", "ns-1", "es-1", "ns-1", caSecretType),
			},
			wantSecrets:     0,
			wantConfigMaps:  0,
			wantDeployments: 0,
		},
		{
			name: "don't cleanup secrets without policy labels",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-secret",
						Namespace: "ns-1",
					},
				},
			},
			wantSecrets:     1,
			wantConfigMaps:  0,
			wantDeployments: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.objects...)
			esClientProvider := newFakeESClientProvider().Provider

			gc := &GarbageCollector{
				client:           k8sClient,
				esClientProvider: esClientProvider,
				dialer:           &fakeDialer{},
			}

			ctx := context.Background()
			err := gc.DoGarbageCollection(ctx)
			require.NoError(t, err)

			// Check secrets
			var secrets corev1.SecretList
			require.NoError(t, k8sClient.List(ctx, &secrets))
			require.Equal(t, tt.wantSecrets, len(secrets.Items), "unexpected number of secrets")

			// Check configmaps
			var configMaps corev1.ConfigMapList
			require.NoError(t, k8sClient.List(ctx, &configMaps))
			require.Equal(t, tt.wantConfigMaps, len(configMaps.Items), "unexpected number of configmaps")

			// Check deployments
			var deployments appsv1.DeploymentList
			require.NoError(t, k8sClient.List(ctx, &deployments))
			require.Equal(t, tt.wantDeployments, len(deployments.Items), "unexpected number of deployments")
		})
	}
}

func TestPolicyFromLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   types.NamespacedName
	}{
		{
			name: "both labels present",
			labels: map[string]string{
				PolicyNameLabelKey:      "my-policy",
				policyNamespaceLabelKey: "my-namespace",
			},
			want: types.NamespacedName{Name: "my-policy", Namespace: "my-namespace"},
		},
		{
			name: "missing name label",
			labels: map[string]string{
				policyNamespaceLabelKey: "my-namespace",
			},
			want: types.NamespacedName{},
		},
		{
			name: "missing namespace label",
			labels: map[string]string{
				PolicyNameLabelKey: "my-policy",
			},
			want: types.NamespacedName{},
		},
		{
			name:   "no labels",
			labels: map[string]string{},
			want:   types.NamespacedName{},
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policyFromLabels(tt.labels)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestESFromLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   types.NamespacedName
	}{
		{
			name: "both labels present",
			labels: map[string]string{
				commonapikey.MetadataKeyESName:      "my-es",
				commonapikey.MetadataKeyESNamespace: "my-namespace",
			},
			want: types.NamespacedName{Name: "my-es", Namespace: "my-namespace"},
		},
		{
			name: "missing name label",
			labels: map[string]string{
				commonapikey.MetadataKeyESNamespace: "my-namespace",
			},
			want: types.NamespacedName{},
		},
		{
			name: "missing namespace label",
			labels: map[string]string{
				commonapikey.MetadataKeyESName: "my-es",
			},
			want: types.NamespacedName{},
		},
		{
			name:   "no labels",
			labels: map[string]string{},
			want:   types.NamespacedName{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := esFromLabels(tt.labels)
			require.Equal(t, tt.want, got)
		})
	}
}
