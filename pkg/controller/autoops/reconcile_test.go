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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
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
			name: "config secret not found sets error phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "missing-secret",
						},
					},
					ResourceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "elasticsearch"},
					},
				},
			},
			initialObjects: []client.Object{},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Phase: autoopsv1alpha1.ErrorPhase,
			},
			wantResults: reconcile.Result{},
		},
		{
			name: "config secret missing required keys sets error phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.1.0",
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "invalid-secret",
						},
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
				Phase: autoopsv1alpha1.ErrorPhase,
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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
					Config: commonv1.ConfigSource{
						SecretRef: commonv1.SecretRef{
							SecretName: "config-secret",
						},
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

			r := &AutoOpsAgentPolicyReconciler{
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
			require.Equal(t, tt.wantStatus.Phase == autoopsv1alpha1.ErrorPhase, gotErr != nil)

			if !cmp.Equal(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")) {
				t.Errorf("status mismatch:\n%s", cmp.Diff(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")))
			}
			require.Equal(t, tt.wantResults, gotResult)
		})
	}
}
