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

	// Initial setup: policy with selector matching es-1 and es-2
	policy := autoopsv1alpha1.AutoOpsAgentPolicy{
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

	// Initial objects: es-1, es-2, es-3, and config secret
	initialObjects := []client.Object{
		configSecret,
		&es1,
		&es2,
		&es3,
	}

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

	// Step 1: Initial reconcile with selector matching es-1 and es-2
	t.Run("initial reconcile creates all resources", func(t *testing.T) {
		state := newState(policy)
		results := reconciler.NewResult(ctx)
		gotResults := r.internalReconcile(ctx, policy, results, state)

		gotResult, gotErr := gotResults.Aggregate()
		require.NoError(t, gotErr)
		require.Equal(t, reconcile.Result{}, gotResult)

		// Verify resources were created for es-1, es-2, and es-3 (all match the initial selector)
		var deployments appsv1.DeploymentList
		require.NoError(t, k8sClient.List(ctx, &deployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, deployments.Items, 3, "Expected 3 deployments for es-1, es-2, and es-3")

		var configMaps corev1.ConfigMapList
		require.NoError(t, k8sClient.List(ctx, &configMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, configMaps.Items, 3, "Expected 3 configmaps for es-1, es-2, and es-3")

		var secrets corev1.SecretList
		require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, secrets.Items, 3, "Expected 3 secrets for es-1, es-2, and es-3")
	})

	// Step 2: Update selector to match only es-3
	updatedPolicy := policy.DeepCopy()
	updatedPolicy.Spec.ResourceSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "elasticsearch", "env": "prod"},
	}

	t.Run("selector change removes old resources and leaves existing matching resources", func(t *testing.T) {
		state := newState(*updatedPolicy)
		results := reconciler.NewResult(ctx)
		gotResults := r.internalReconcile(ctx, *updatedPolicy, results, state)

		gotResult, gotErr := gotResults.Aggregate()
		require.NoError(t, gotErr)
		require.Equal(t, reconcile.Result{}, gotResult)

		// Verify old resources (es-1, es-2) are deleted
		es1DeploymentName := autoopsv1alpha1.Deployment("policy-1", es1)
		var es1Deployment appsv1.Deployment
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es1DeploymentName}, &es1Deployment)
		require.True(t, apierrors.IsNotFound(err), "es-1 deployment should be deleted")

		es2DeploymentName := autoopsv1alpha1.Deployment("policy-1", es2)
		var es2Deployment appsv1.Deployment
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2DeploymentName}, &es2Deployment)
		require.True(t, apierrors.IsNotFound(err), "es-2 deployment should be deleted")

		es1ConfigMapName := autoopsv1alpha1.Config("policy-1", es1)
		var es1ConfigMap corev1.ConfigMap
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es1ConfigMapName}, &es1ConfigMap)
		require.True(t, apierrors.IsNotFound(err), "es-1 configmap should be deleted")

		es2ConfigMapName := autoopsv1alpha1.Config("policy-1", es2)
		var es2ConfigMap corev1.ConfigMap
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2ConfigMapName}, &es2ConfigMap)
		require.True(t, apierrors.IsNotFound(err), "es-2 configmap should be deleted")

		es1APIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: "es-1", Namespace: "ns-1"})
		var es1APIKeySecret corev1.Secret
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es1APIKeySecretName}, &es1APIKeySecret)
		require.True(t, apierrors.IsNotFound(err), "es-1 API key secret should be deleted")

		es2APIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: "es-2", Namespace: "ns-1"})
		var es2APIKeySecret corev1.Secret
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2APIKeySecretName}, &es2APIKeySecret)
		require.True(t, apierrors.IsNotFound(err), "es-2 API key secret should be deleted")

		// Verify resource for es-3 still exist.
		es3DeploymentName := autoopsv1alpha1.Deployment("policy-1", es3)
		var es3Deployment appsv1.Deployment
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es3DeploymentName}, &es3Deployment)
		require.NoError(t, err, "es-3 deployment should still exist")
		require.Equal(t, "es-3", es3Deployment.Labels[commonapikey.MetadataKeyESName])
		require.Equal(t, "ns-1", es3Deployment.Labels[commonapikey.MetadataKeyESNamespace])

		es3ConfigMapName := autoopsv1alpha1.Config("policy-1", es3)
		var es3ConfigMap corev1.ConfigMap
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es3ConfigMapName}, &es3ConfigMap)
		require.NoError(t, err, "es-3 configmap should still exist")
		require.Equal(t, "es-3", es3ConfigMap.Labels[commonapikey.MetadataKeyESName])
		require.Equal(t, "ns-1", es3ConfigMap.Labels[commonapikey.MetadataKeyESNamespace])

		es3APIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: "es-3", Namespace: "ns-1"})
		var es3APIKeySecret corev1.Secret
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es3APIKeySecretName}, &es3APIKeySecret)
		require.NoError(t, err, "es-3 API key secret should still exist")
		require.Equal(t, "es-3", es3APIKeySecret.Labels[commonapikey.MetadataKeyESName])
		require.Equal(t, "ns-1", es3APIKeySecret.Labels[commonapikey.MetadataKeyESNamespace])

		// Verify no additional resources were created.
		var deployments appsv1.DeploymentList
		require.NoError(t, k8sClient.List(ctx, &deployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, deployments.Items, 1, "Should have exactly 1 deployment after selector change")

		var configMaps corev1.ConfigMapList
		require.NoError(t, k8sClient.List(ctx, &configMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, configMaps.Items, 1, "Should have exactly 1 configmap after selector change")

		var secrets corev1.SecretList
		require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, secrets.Items, 1, "Should have exactly 1 secret after selector change")
	})
}
