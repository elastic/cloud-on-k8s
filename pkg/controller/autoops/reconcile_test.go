// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestAutoOpsAgentPolicyReconciler_internalReconcile(t *testing.T) {
	scheme.SetupScheme()

	tests := []struct {
		name             string
		policy           autoopsv1alpha1.AutoOpsAgentPolicy
		initialObjects   []client.Object
		esClientProvider commonesclient.Provider
		wantStatus       autoopsv1alpha1.AutoOpsAgentPolicyStatus
		wantResults      reconcile.Result
	}{
		{
			name: "config secret not found sets invalid phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "missing-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Phase: autoopsv1alpha1.InvalidPhase,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "config secret missing required keys sets invalid phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "invalid-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Phase: autoopsv1alpha1.InvalidPhase,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "invalid label selector sets error phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "app",
								Operator: "InvalidOperator",
								Values:   []string{"elasticsearch"},
							},
						},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Phase: autoopsv1alpha1.ErrorPhase,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "no ES resources found sets no resources phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Phase:     autoopsv1alpha1.NoResourcesPhase,
				Resources: 0,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "ES resource not ready",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchApplyingChangesPhase,
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 1,
				Ready:     0,
				Phase:     autoopsv1alpha1.ResourcesNotReadyPhase,
			},
			wantResults: reconcile.Result{RequeueAfter: 5 * time.Second},
		},
		{
			name: "successful reconciliation with one ready ES",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 1,
				Ready:     0, // Deployment won't be ready yet.
				Errors:    0,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "multiple ES resources with mixed states",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-2",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchApplyingChangesPhase,
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 2,
				Ready:     0, // Deployment won't be ready yet.
				Errors:    0,
				Phase:     autoopsv1alpha1.ResourcesNotReadyPhase, // es-2 is not ready
			},
			wantResults: reconcile.Result{RequeueAfter: 5 * time.Second},
		},
		{
			name: "single ES with ready deployment shows ready: 1, resources: 1",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.Config("policy-1", esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns-1"}}),
						Namespace: "ns-1",
					},
					Data: map[string]string{
						autoOpsESConfigFileName: "test-config",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: "es-1", Namespace: "ns-1"}),
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-api-key"),
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.Deployment("policy-1", esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns-1"}}),
						Namespace: "ns-1",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 1,
				Ready:     1,
				Errors:    0,
				Phase:     "", // Ready, or applyingChanges phases are set in the main Reconcile function, not here.
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "two ES instances: one with ready deployment, one without shows ready: 1, resources: 2",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
						SecretName: "config-secret",
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config-secret",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"cloud-connected-mode-api-key": []byte("test-key"),
						"autoops-otel-url":             []byte("https://test-url"),
						"autoops-token":                []byte("test-token"),
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-2",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
						HTTP: commonv1.HTTPConfig{
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.Config("policy-1", esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns-1"}}),
						Namespace: "ns-1",
					},
					Data: map[string]string{
						autoOpsESConfigFileName: "test-config",
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: "es-1", Namespace: "ns-1"}),
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						apiKeySecretKey: []byte("test-api-key"),
					},
				},
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      autoopsv1alpha1.Deployment("policy-1", esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es-1", Namespace: "ns-1"}}),
						Namespace: "ns-1",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 2,
				Ready:     1,
				Errors:    0,
				Phase:     "", // Ready, or applyingChanges phases are set in the main Reconcile function, not here.
			},
			wantResults: reconcile.Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.initialObjects...)
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
				licenseChecker: license.NewLicenseChecker(k8sClient, "test-namespace"),
			}

			ctx := context.Background()
			state := newState(tt.policy)
			results := reconciler.NewResult(ctx)

			gotResults := r.internalReconcile(ctx, tt.policy, results, state)

			gotResult, gotErr := gotResults.Aggregate()
			expectError := tt.wantStatus.Phase == autoopsv1alpha1.ErrorPhase || tt.wantStatus.Phase == autoopsv1alpha1.InvalidPhase
			require.Equal(t, expectError, gotErr != nil)

			if !cmp.Equal(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")) {
				t.Errorf("status mismatch:\n%s", cmp.Diff(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")))
			}
			require.Equal(t, tt.wantResults, gotResult)
		})
	}
}

func TestAutoOpsAgentPolicyReconciler_selectorChangeCleanup(t *testing.T) {
	scheme.SetupScheme()

	// Common test fixtures
	es1 := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-1",
			Namespace: "ns-1",
			Labels:    map[string]string{"app": "elasticsearch"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.1.0",
		},
		Status: esv1.ElasticsearchStatus{
			Phase: esv1.ElasticsearchReadyPhase,
		},
	}

	es2 := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-2",
			Namespace: "ns-1",
			Labels:    map[string]string{"app": "elasticsearch"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.1.0",
		},
		Status: esv1.ElasticsearchStatus{
			Phase: esv1.ElasticsearchReadyPhase,
		},
	}

	es3 := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-3",
			Namespace: "ns-1",
			Labels:    map[string]string{"app": "elasticsearch", "env": "prod"},
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.1.0",
		},
		Status: esv1.ElasticsearchStatus{
			Phase: esv1.ElasticsearchReadyPhase,
		},
	}

	configSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config-secret",
			Namespace: "ns-1",
		},
		Data: map[string][]byte{
			"cloud-connected-mode-api-key": []byte("test-key"),
			"autoops-otel-url":             []byte("https://test-url"),
			"autoops-token":                []byte("test-token"),
		},
	}

	initialObjects := []client.Object{
		configSecret,
		&es1,
		&es2,
		&es3,
	}

	initialPolicy := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "ns-1",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: "9.1.0",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
				SecretName: "config-secret",
			},
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
		},
	}

	tests := []struct {
		name                string
		updatedSelector     metav1.LabelSelector
		expectedDeployments int
		expectedConfigMaps  int
		expectedSecrets     int
		shouldExist         []esv1.Elasticsearch
		shouldNotExist      []esv1.Elasticsearch
	}{
		{
			name: "selector change from matching 3 instances to matching 1 instance should cleanup 2 instances",
			updatedSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch", "env": "prod"},
			},
			expectedDeployments: 1,
			expectedConfigMaps:  1,
			expectedSecrets:     1,
			shouldExist:         []esv1.Elasticsearch{es3},
			shouldNotExist:      []esv1.Elasticsearch{es1, es2},
		},
		{
			name: "selector change from matching 3 instances to matching none should cleanup all 3 instances",
			updatedSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nonexistent"},
			},
			expectedDeployments: 0,
			expectedConfigMaps:  0,
			expectedSecrets:     0,
			shouldExist:         []esv1.Elasticsearch{},
			shouldNotExist:      []esv1.Elasticsearch{es1, es2, es3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(initialObjects...)
			esClientProvider := newFakeESClientProvider().Provider
			r := &AgentPolicyReconciler{
				Client:           k8sClient,
				esClientProvider: esClientProvider,
				params: operator.Parameters{
					Dialer: &fakeDialer{},
				},
				dynamicWatches: watches.NewDynamicWatches(),
				licenseChecker: license.NewLicenseChecker(k8sClient, "test-namespace"),
			}

			ctx := context.Background()

			// Initial reconcile with selector matching es-1, es-2, and es-3
			state := newState(initialPolicy)
			results := reconciler.NewResult(ctx)
			gotResults := r.internalReconcile(ctx, initialPolicy, results, state)

			gotResult, gotErr := gotResults.Aggregate()
			require.NoError(t, gotErr)
			require.Equal(t, reconcile.Result{}, gotResult)

			// Verify resources were created for all ES instances that match the initial selector
			var deployments appsv1.DeploymentList
			require.NoError(t, k8sClient.List(ctx, &deployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, deployments.Items, 3, "Expected 3 deployments for es-1, es-2, and es-3")

			var configMaps corev1.ConfigMapList
			require.NoError(t, k8sClient.List(ctx, &configMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, configMaps.Items, 3, "Expected 3 configmaps for es-1, es-2, and es-3")

			var secrets corev1.SecretList
			require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, secrets.Items, 3, "Expected 3 secrets for es-1, es-2, and es-3")

			// Update selector and re-reconcile
			updatedPolicy := initialPolicy.DeepCopy()
			updatedPolicy.Spec.ResourceSelector = tt.updatedSelector

			state = newState(*updatedPolicy)
			results = reconciler.NewResult(ctx)
			gotResults = r.internalReconcile(ctx, *updatedPolicy, results, state)

			gotResult, gotErr = gotResults.Aggregate()
			require.NoError(t, gotErr)
			require.Equal(t, reconcile.Result{}, gotResult)

			// Verify resources that should not exist are deleted
			for _, es := range tt.shouldNotExist {
				esDeploymentName := autoopsv1alpha1.Deployment("policy-1", es)
				var esDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esDeploymentName}, &esDeployment)
				require.True(t, apierrors.IsNotFound(err), "deployment for %s should be deleted", es.Name)

				esConfigMapName := autoopsv1alpha1.Config("policy-1", es)
				var esConfigMap corev1.ConfigMap
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esConfigMapName}, &esConfigMap)
				require.True(t, apierrors.IsNotFound(err), "configmap for %s should be deleted", es.Name)

				esAPIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: es.Name, Namespace: "ns-1"})
				var esAPIKeySecret corev1.Secret
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esAPIKeySecretName}, &esAPIKeySecret)
				require.True(t, apierrors.IsNotFound(err), "API key secret for %s should be deleted", es.Name)
			}

			// Verify resources that should exist still exist
			for _, es := range tt.shouldExist {
				esDeploymentName := autoopsv1alpha1.Deployment("policy-1", es)
				var esDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esDeploymentName}, &esDeployment)
				require.NoError(t, err, "deployment for %s should still exist", es.Name)
				require.Equal(t, es.Name, esDeployment.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, "ns-1", esDeployment.Labels[commonapikey.MetadataKeyESNamespace])

				esConfigMapName := autoopsv1alpha1.Config("policy-1", es)
				var esConfigMap corev1.ConfigMap
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esConfigMapName}, &esConfigMap)
				require.NoError(t, err, "configmap for %s should still exist", es.Name)
				require.Equal(t, es.Name, esConfigMap.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, "ns-1", esConfigMap.Labels[commonapikey.MetadataKeyESNamespace])

				esAPIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: es.Name, Namespace: "ns-1"})
				var esAPIKeySecret corev1.Secret
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: esAPIKeySecretName}, &esAPIKeySecret)
				require.NoError(t, err, "API key secret for %s should still exist", es.Name)
				require.Equal(t, es.Name, esAPIKeySecret.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, "ns-1", esAPIKeySecret.Labels[commonapikey.MetadataKeyESNamespace])
			}

			// Verify eventual expected resource counts
			var finalDeployments appsv1.DeploymentList
			require.NoError(t, k8sClient.List(ctx, &finalDeployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalDeployments.Items, tt.expectedDeployments, "Should have exactly %d deployment(s) after selector change", tt.expectedDeployments)

			var finalConfigMaps corev1.ConfigMapList
			require.NoError(t, k8sClient.List(ctx, &finalConfigMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalConfigMaps.Items, tt.expectedConfigMaps, "Should have exactly %d configmap(s) after selector change", tt.expectedConfigMaps)

			var finalSecrets corev1.SecretList
			require.NoError(t, k8sClient.List(ctx, &finalSecrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalSecrets.Items, tt.expectedSecrets, "Should have exactly %d secret(s) after selector change", tt.expectedSecrets)
		})
	}
}
