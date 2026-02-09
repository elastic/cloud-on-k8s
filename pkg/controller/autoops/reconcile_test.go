// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"errors"
	"strings"
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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
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
					Version: "9.2.1",
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
					Version: "9.2.1",
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
			name: "no ES resources found sets no monitored resources phase",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
				Phase:     autoopsv1alpha1.NoMonitoredResourcesPhase,
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
					Version: "9.2.1",
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
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 1,
				Ready:     0,
				Phase:     autoopsv1alpha1.MonitoredResourcesNotReadyPhase,
			},
			wantResults: reconcile.Result{RequeueAfter: 10 * time.Second},
		},
		{
			name: "successful reconciliation with one ready ES",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					Version: "9.2.1",
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
				Phase:     autoopsv1alpha1.MonitoredResourcesNotReadyPhase, // es-2 is not ready
			},
			wantResults: reconcile.Result{RequeueAfter: 10 * time.Second},
		},
		{
			name: "single ES with ready deployment shows ready: 1, resources: 1",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					Version: "9.2.1",
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
		{
			name: "deprecated ES version is ignored",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
						Name:      "es-deprecated",
						Namespace: "ns-1",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "7.15.0",
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Resources: 1,
				Ready:     0,
				Errors:    0,
				Phase:     "",
			},
		},
		{
			name: "two ES instances: filter by namespace shows ready: 1, resources: 1",
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
					NamespaceSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ns-1"},
					},
				},
			},
			initialObjects: []client.Object{
				&corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-1",
						Labels: map[string]string{
							"kubernetes.io/metadata.name": "ns-1",
						},
					},
				},
				&corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "ns-2",
						Labels: map[string]string{
							"kubernetes.io/metadata.name": "ns-2",
						},
					},
				},
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				&esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-2",
						Namespace: "ns-2",
						Labels:    map[string]string{"app": "elasticsearch"},
					},
					Spec: esv1.ElasticsearchSpec{
						Version: "9.1.0",
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
				accessReviewer:   &fakeAccessReviewer{allowed: true},
				esClientProvider: esClientProvider,
				params: operator.Parameters{
					Dialer: &fakeDialer{},
				},
				dynamicWatches: watches.NewDynamicWatches(),
			}

			ctx := context.Background()
			state := newState(tt.policy)
			results := reconciler.NewResult(ctx)

			gotResults := r.internalReconcile(ctx, tt.policy, results, state)

			gotResult, gotErr := gotResults.Aggregate()
			expectError := tt.wantStatus.Phase == autoopsv1alpha1.ErrorPhase || tt.wantStatus.Phase == autoopsv1alpha1.InvalidPhase
			require.Equal(t, expectError, gotErr != nil, "expected error: %v, got error: %v", expectError, gotErr)

			// avoid having to initialize it in every testcase entry.
			if tt.wantStatus.Details == nil {
				tt.wantStatus.Details = map[string]autoopsv1alpha1.AutoOpsResourceStatus{}
			}

			if !cmp.Equal(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")) {
				t.Errorf("status mismatch:\n%s", cmp.Diff(tt.wantStatus, state.status, cmpopts.IgnoreFields(autoopsv1alpha1.AutoOpsAgentPolicyStatus{}, "ObservedGeneration")))
			}
			require.Equal(t, tt.wantResults, gotResult)
		})
	}
}

func TestAutoOpsAgentPolicyReconciler_internalReconcileResourceErrors(t *testing.T) {
	scheme.SetupScheme()

	tests := []struct {
		name             string
		policy           autoopsv1alpha1.AutoOpsAgentPolicy
		initialObjects   []client.Object
		accessReviewer   *fakeAccessReviewer
		esClientProvider commonesclient.Provider
		interceptorFuncs *interceptor.Funcs
		wantErr          bool
		wantStatus       autoopsv1alpha1.AutoOpsAgentPolicyStatus
	}{
		{
			name: "accessReviewer error sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version:            "9.2.1",
					ServiceAccountName: "test-sa",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{err: errors.New("access review failed")},
			wantErr:        true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed trying to perform RBAC check: access review failed",
					},
				},
			},
		},
		{
			name: "rbac not allowed sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version:            "9.2.1",
					ServiceAccountName: "test-sa",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: false},
			wantErr:        false,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Skipped: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase:   autoopsv1alpha1.SkippedResourcePhase,
						Message: "RBAC access denied for service account test-sa",
					},
				},
			},
		},
		{
			name: "reconcileAutoOpsESCASecret failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
						// TLS is enabled by default (SelfSignedCertificate is nil)
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
				// The ES http-certs-public secret that contains the CA certificate
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "es-1-es-http-certs-public",
						Namespace: "ns-1",
					},
					Data: map[string][]byte{
						"tls.crt": []byte("test-ca-cert"),
						"ca.crt":  []byte("test-ca-cert"),
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			interceptorFuncs: &interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if s, ok := obj.(*corev1.Secret); ok && strings.HasPrefix(s.Name, "policy-1-autoops-ca") {
						return errors.New("secret creation failed")
					}
					return client.Create(ctx, obj, opts...)
				},
			},
			wantErr: true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to create AutoOps ES CA secret: secret creation failed",
					},
				},
			},
		},
		{
			name: "reconcileAutoOpsESAPIKey failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			esClientProvider: newFakeESClientProviderWithClient(&fakeESClient{
				getAPIKeysByNameErr: errors.New("elasticsearch API error"),
			}).Provider,
			wantErr: true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to create AutoOps ES API key: while getting API keys by name autoops-ns-1-policy-1-ns-1-es-1: elasticsearch API error",
					},
				},
			},
		},
		{
			name: "ReconcileAutoOpsESConfigMap failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			interceptorFuncs: &interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if _, ok := obj.(*corev1.ConfigMap); ok {
						return errors.New("configmap creation failed")
					}
					return client.Create(ctx, obj, opts...)
				},
			},
			wantErr: true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to create AutoOps ES config map: configmap creation failed",
					},
				},
			},
		},
		{
			name: "buildConfigHash failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			interceptorFuncs: func() *interceptor.Funcs {
				// Track calls to config-secret to fail only on buildConfigHash (second call)
				configSecretGetCount := 0
				return &interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						if _, ok := obj.(*corev1.Secret); ok && key.Name == "config-secret" && key.Namespace == "ns-1" {
							configSecretGetCount++
							// First call is from validateConfigSecret, let it pass
							// Second call is from buildConfigHash, fail it
							if configSecretGetCount > 1 {
								return errors.New("config secret access failed during hash computation")
							}
						}
						return client.Get(ctx, key, obj, opts...)
					},
				}
			}(),
			wantErr: true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to prepare AutoOps agent deployment: while getting autoops configuration secret ns-1/config-secret: config secret access failed during hash computation",
					},
				},
			},
		},
		{
			name: "buildDeployment failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "invalid-version", // Invalid version will cause buildDeployment to fail
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			wantErr:        true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to build AutoOps agent deployment: No Major.Minor.Patch elements found",
					},
				},
			},
		},
		{
			name: "deployment.Reconcile failure sets correct status Details",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					Version: "9.2.1",
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
					},
					Status: esv1.ElasticsearchStatus{
						Phase: esv1.ElasticsearchReadyPhase,
					},
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			interceptorFuncs: &interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if d, ok := obj.(*appsv1.Deployment); ok && strings.HasPrefix(d.Name, "policy-1-autoops-deploy") {
						return errors.New("deployment creation failed")
					}
					return client.Create(ctx, obj, opts...)
				},
			},
			wantErr: true,
			wantStatus: autoopsv1alpha1.AutoOpsAgentPolicyStatus{
				Errors: 1,
				Details: map[string]autoopsv1alpha1.AutoOpsResourceStatus{
					"ns-1/es-1": {
						Phase: autoopsv1alpha1.ErrorResourcePhase,
						Error: "Failed to reconcile AutoOps agent deployment: deployment creation failed",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esClientProvider := tt.esClientProvider
			if esClientProvider == nil {
				esClientProvider = newFakeESClientProvider().Provider
			}

			var k8sClient k8s.Client
			if tt.interceptorFuncs == nil {
				k8sClient = k8s.NewFakeClient(tt.initialObjects...)
			} else {
				k8sClient = k8s.NewFakeClientBuilder(tt.initialObjects...).
					WithInterceptorFuncs(*tt.interceptorFuncs).
					Build()
			}

			r := &AgentPolicyReconciler{
				Client:           k8sClient,
				esClientProvider: esClientProvider,
				accessReviewer:   tt.accessReviewer,
				recorder:         record.NewFakeRecorder(10),
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

			_, gotErr := gotResults.Aggregate()
			if tt.wantErr {
				require.Error(t, gotErr, "expected error")
			} else {
				require.NoError(t, gotErr, "unexpected error")
			}

			// Verify Skipped count
			require.Equal(t, tt.wantStatus.Skipped, state.status.Skipped, "Skipped count mismatch")

			// Verify Errors count
			require.Equal(t, tt.wantStatus.Errors, state.status.Errors, "Errors count mismatch")

			// Verify Details field
			require.Len(t, state.status.Details, len(tt.wantStatus.Details))
			require.Equal(t, tt.wantStatus.Details, state.status.Details)
		})
	}
}

func TestAutoOpsAgentPolicyReconciler_selectorChangeCleanup(t *testing.T) {
	scheme.SetupScheme()

	// Common test fixtures
	ns1 := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns-1",
			Labels: map[string]string{
				"kubernetes.io/metadata.name": "ns-1",
			},
		},
	}
	ns2 := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns-2",
			Labels: map[string]string{
				"kubernetes.io/metadata.name": "ns-2",
			},
		},
	}
	ns3 := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ns-3",
			Labels: map[string]string{
				"kubernetes.io/metadata.name": "ns-3",
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
			Namespace: "ns-2",
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
		&ns1,
		&ns2,
		&ns3,
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
			Version: "9.2.1",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
				SecretName: "config-secret",
			},
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
		},
	}

	tests := []struct {
		name                     string
		updatedResourceSelector  metav1.LabelSelector
		updatedNamespaceSelector metav1.LabelSelector
		expectedDeployments      int
		expectedConfigMaps       int
		expectedSecrets          int
		shouldExist              []esv1.Elasticsearch
		shouldNotExist           []esv1.Elasticsearch
	}{
		{
			name: "selector change from matching 3 instances to matching 1 instance should cleanup 2 instances",
			updatedResourceSelector: metav1.LabelSelector{
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
			updatedResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nonexistent"},
			},
			expectedDeployments: 0,
			expectedConfigMaps:  0,
			expectedSecrets:     0,
			shouldExist:         []esv1.Elasticsearch{},
			shouldNotExist:      []esv1.Elasticsearch{es1, es2, es3},
		},
		{
			name: "namespace selector change from matching 3 instances to matching 1 instance should cleanup 2 instances",
			updatedNamespaceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ns-2"},
			},
			expectedDeployments: 1,
			expectedConfigMaps:  1,
			expectedSecrets:     1,
			shouldExist:         []esv1.Elasticsearch{es3},
			shouldNotExist:      []esv1.Elasticsearch{es1, es2},
		},
		{
			name: "namespace selector change from matching 3 instances to matching none should cleanup all 3 instances",
			updatedNamespaceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ns-3"},
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
				accessReviewer:   &fakeAccessReviewer{allowed: true},
				esClientProvider: esClientProvider,
				params: operator.Parameters{
					Dialer: &fakeDialer{},
				},
				dynamicWatches: watches.NewDynamicWatches(),
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
			require.NoError(t, k8sClient.List(ctx, &deployments, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, deployments.Items, 3, "Expected 3 deployments for es-1, es-2, and es-3")

			var configMaps corev1.ConfigMapList
			require.NoError(t, k8sClient.List(ctx, &configMaps, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, configMaps.Items, 3, "Expected 3 configmaps for es-1, es-2, and es-3")

			var secrets corev1.SecretList
			require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, secrets.Items, 3, "Expected 3 secrets for es-1, es-2, and es-3")

			// Update selector and re-reconcile
			updatedPolicy := initialPolicy.DeepCopy()
			if tt.updatedResourceSelector.Size() > 0 {
				updatedPolicy.Spec.ResourceSelector = tt.updatedResourceSelector
			}
			if tt.updatedNamespaceSelector.Size() > 0 {
				updatedPolicy.Spec.NamespaceSelector = tt.updatedNamespaceSelector
			}

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
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esDeploymentName}, &esDeployment)
				require.True(t, apierrors.IsNotFound(err), "deployment for %s should be deleted", es.Name)

				esConfigMapName := autoopsv1alpha1.Config("policy-1", es)
				var esConfigMap corev1.ConfigMap
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esConfigMapName}, &esConfigMap)
				require.True(t, apierrors.IsNotFound(err), "configmap for %s should be deleted", es.Name)

				esAPIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: es.Name, Namespace: ns1.Name})
				var esAPIKeySecret corev1.Secret
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esAPIKeySecretName}, &esAPIKeySecret)
				require.True(t, apierrors.IsNotFound(err), "API key secret for %s should be deleted", es.Name)
			}

			// Verify resources that should exist still exist
			for _, es := range tt.shouldExist {
				esDeploymentName := autoopsv1alpha1.Deployment("policy-1", es)
				var esDeployment appsv1.Deployment
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esDeploymentName}, &esDeployment)
				require.NoError(t, err, "deployment for %s should still exist", es.Name)
				require.Equal(t, es.Name, esDeployment.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, es.Namespace, esDeployment.Labels[commonapikey.MetadataKeyESNamespace])

				esConfigMapName := autoopsv1alpha1.Config("policy-1", es)
				var esConfigMap corev1.ConfigMap
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esConfigMapName}, &esConfigMap)
				require.NoError(t, err, "configmap for %s should still exist", es.Name)
				require.Equal(t, es.Name, esConfigMap.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, es.Namespace, esConfigMap.Labels[commonapikey.MetadataKeyESNamespace])

				esAPIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: es.Name, Namespace: es.Namespace})
				var esAPIKeySecret corev1.Secret
				err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns1.Name, Name: esAPIKeySecretName}, &esAPIKeySecret)
				require.NoError(t, err, "API key secret for %s should still exist", es.Name)
				require.Equal(t, es.Name, esAPIKeySecret.Labels[commonapikey.MetadataKeyESName])
				require.Equal(t, es.Namespace, esAPIKeySecret.Labels[commonapikey.MetadataKeyESNamespace])
			}

			// Verify eventual expected resource counts
			var finalDeployments appsv1.DeploymentList
			require.NoError(t, k8sClient.List(ctx, &finalDeployments, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalDeployments.Items, tt.expectedDeployments, "Should have exactly %d deployment(s) after selector change", tt.expectedDeployments)

			var finalConfigMaps corev1.ConfigMapList
			require.NoError(t, k8sClient.List(ctx, &finalConfigMaps, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalConfigMaps.Items, tt.expectedConfigMaps, "Should have exactly %d configmap(s) after selector change", tt.expectedConfigMaps)

			var finalSecrets corev1.SecretList
			require.NoError(t, k8sClient.List(ctx, &finalSecrets, client.InNamespace(ns1.Name), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
			require.Len(t, finalSecrets.Items, tt.expectedSecrets, "Should have exactly %d secret(s) after selector change", tt.expectedSecrets)
		})
	}
}

func TestAutoOpsAgentPolicyReconciler_accessRevokedCleanup(t *testing.T) {
	scheme.SetupScheme()

	// ES clusters that will be used in the test
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

	policy := autoopsv1alpha1.AutoOpsAgentPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "ns-1",
		},
		Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
			Version: "9.2.1",
			AutoOpsRef: autoopsv1alpha1.AutoOpsRef{
				SecretName: "config-secret",
			},
			ResourceSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "elasticsearch"},
			},
		},
	}

	initialObjects := []client.Object{
		configSecret,
		&es1,
		&es2,
	}

	t.Run("resources are cleaned up when access is revoked", func(t *testing.T) {
		k8sClient := k8s.NewFakeClient(initialObjects...)
		esClientProvider := newFakeESClientProvider().Provider

		// Start with access allowed to both clusters
		accessReviewer := newConfigurableAccessReviewer(true)

		r := &AgentPolicyReconciler{
			Client:           k8sClient,
			esClientProvider: esClientProvider,
			accessReviewer:   accessReviewer,
			recorder:         record.NewFakeRecorder(10),
			params: operator.Parameters{
				Dialer: &fakeDialer{},
			},
			dynamicWatches: watches.NewDynamicWatches(),
		}

		ctx := context.Background()

		// First reconcile with access to both clusters
		state := newState(policy)
		results := reconciler.NewResult(ctx)
		gotResults := r.internalReconcile(ctx, policy, results, state)

		_, gotErr := gotResults.Aggregate()
		require.NoError(t, gotErr)

		// Verify resources were created for both ES instances
		var deployments appsv1.DeploymentList
		require.NoError(t, k8sClient.List(ctx, &deployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, deployments.Items, 2, "Expected 2 deployments for es-1 and es-2")

		var configMaps corev1.ConfigMapList
		require.NoError(t, k8sClient.List(ctx, &configMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, configMaps.Items, 2, "Expected 2 configmaps for es-1 and es-2")

		var secrets corev1.SecretList
		require.NoError(t, k8sClient.List(ctx, &secrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, secrets.Items, 2, "Expected 2 secrets for es-1 and es-2")

		// Now revoke access to es-2
		accessReviewer.SetAccess("ns-1", "es-2", false)

		// Re-reconcile
		state = newState(policy)
		results = reconciler.NewResult(ctx)
		gotResults = r.internalReconcile(ctx, policy, results, state)

		_, gotErr = gotResults.Aggregate()
		require.NoError(t, gotErr)

		// Verify resources for es-2 were cleaned up
		var finalDeployments appsv1.DeploymentList
		require.NoError(t, k8sClient.List(ctx, &finalDeployments, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, finalDeployments.Items, 1, "Should have exactly 1 deployment after access revoked")

		var finalConfigMaps corev1.ConfigMapList
		require.NoError(t, k8sClient.List(ctx, &finalConfigMaps, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, finalConfigMaps.Items, 1, "Should have exactly 1 configmap after access revoked")

		var finalSecrets corev1.SecretList
		require.NoError(t, k8sClient.List(ctx, &finalSecrets, client.InNamespace("ns-1"), client.MatchingLabels{PolicyNameLabelKey: "policy-1"}))
		require.Len(t, finalSecrets.Items, 1, "Should have exactly 1 secret after access revoked")

		// Verify es-1 resources still exist
		es1DeploymentName := autoopsv1alpha1.Deployment("policy-1", es1)
		var es1Deployment appsv1.Deployment
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es1DeploymentName}, &es1Deployment)
		require.NoError(t, err, "deployment for es-1 should still exist")

		// Verify es-2 resources are deleted
		es2DeploymentName := autoopsv1alpha1.Deployment("policy-1", es2)
		var es2Deployment appsv1.Deployment
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2DeploymentName}, &es2Deployment)
		require.True(t, apierrors.IsNotFound(err), "deployment for es-2 should be deleted after access revoked")

		es2ConfigMapName := autoopsv1alpha1.Config("policy-1", es2)
		var es2ConfigMap corev1.ConfigMap
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2ConfigMapName}, &es2ConfigMap)
		require.True(t, apierrors.IsNotFound(err), "configmap for es-2 should be deleted after access revoked")

		es2APIKeySecretName := autoopsv1alpha1.APIKeySecret("policy-1", types.NamespacedName{Name: es2.Name, Namespace: "ns-1"})
		var es2APIKeySecret corev1.Secret
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "ns-1", Name: es2APIKeySecretName}, &es2APIKeySecret)
		require.True(t, apierrors.IsNotFound(err), "API key secret for es-2 should be deleted after access revoked")

		// Verify status reflects only the accessible cluster
		require.Equal(t, 1, state.status.Resources, "Resources count should be 1 after access revoked")
	})
}
