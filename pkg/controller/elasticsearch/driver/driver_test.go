// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func Test_esReachableConditionMessage(t *testing.T) {
	type args struct {
		internalService        *corev1.Service
		isServiceReady         bool
		isRespondingToRequests bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         false,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has no endpoint",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: false,
			},
			want: "Service namespace/name has endpoints but Elasticsearch is unavailable",
		},
		{
			args: args{
				internalService:        &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "namespace"}},
				isServiceReady:         true,
				isRespondingToRequests: true,
			},
			want: "Service namespace/name has endpoints",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := esReachableConditionMessage(tt.args.internalService, tt.args.isServiceReady, tt.args.isRespondingToRequests); got != tt.want {
				t.Errorf("esReachableConditionMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_allNodesRunningServiceAccounts(t *testing.T) {
	type args struct {
		saTokens       user.ServiceAccountTokens
		allPods        set.StringSet
		securityClient esclient.SecurityClient
	}
	tests := []struct {
		name    string
		args    args
		want    *bool
		wantErr bool
	}{
		{
			name: "All nodes are running with expected tokens",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make("elasticsearch-sample-es-default-1", "elasticsearch-sample-es-default-0"),
			},
			want: ptr.To[bool](true),
		},
		{
			name: "One node is not running with an expected token",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0"}),
				allPods: set.Make("elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"),
			},
			want: ptr.To[bool](false),
		},
		{
			name: "More nodes running with tokens than expected",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make("elasticsearch-sample-es-default-0"),
			},
			want: ptr.To[bool](true),
		},
		{
			name: "No expected tokens",
			args: args{
				saTokens:       []user.ServiceAccountToken{},
				securityClient: newFakeSecurityClient(),
				allPods:        set.Make("elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-0"),
			},
			want: nil,
		},
		{
			name: "No Pods",
			args: args{
				saTokens: []user.ServiceAccountToken{
					{
						FullyQualifiedServiceAccountName: "elastic/kibana/default_kibana-sample_token1",
					},
				},
				securityClient: newFakeSecurityClient().
					withFileTokens("elastic/kibana", "default_kibana-sample_token1", []string{"elasticsearch-sample-es-default-0", "elasticsearch-sample-es-default-1"}),
				allPods: set.Make(),
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := allNodesRunningServiceAccounts(context.TODO(), tt.args.saTokens, tt.args.allPods, tt.args.securityClient)
			if (err != nil) != tt.wantErr {
				t.Errorf("defaultDriver.isServiceAccountsReady() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

type fakeSecurityClient struct {
	// namespacedService -> ServiceAccountCredential
	serviceAccountCredentials map[string]esclient.ServiceAccountCredential
}

var _ esclient.SecurityClient = &fakeSecurityClient{}

func (f *fakeSecurityClient) GetServiceAccountCredentials(_ context.Context, namespacedService string) (esclient.ServiceAccountCredential, error) {
	serviceAccountCredential := f.serviceAccountCredentials[namespacedService]
	return serviceAccountCredential, nil
}

func newFakeSecurityClient() *fakeSecurityClient {
	return &fakeSecurityClient{
		serviceAccountCredentials: make(map[string]esclient.ServiceAccountCredential),
	}
}

func (f *fakeSecurityClient) withFileTokens(namespacedService, tokenName string, nodes []string) *fakeSecurityClient {
	serviceAccountCredential, exists := f.serviceAccountCredentials[namespacedService]
	if !exists {
		serviceAccountCredential.NodesCredentials = esclient.NodesCredentials{
			FileTokens: make(map[string]esclient.FileToken),
		}
	}

	serviceAccountCredential.NodesCredentials.FileTokens[tokenName] = esclient.FileToken{
		Nodes: nodes,
	}
	f.serviceAccountCredentials[namespacedService] = serviceAccountCredential
	return f
}

func Test_maybeReconcileEmptyFileSettingsSecret(t *testing.T) {
	const operatorNamespace = "elastic-system"

	tests := []struct {
		name              string
		es                *esv1.Elasticsearch
		policies          []policyv1alpha1.StackConfigPolicy
		existingSecrets   []corev1.Secret
		licenseChecker    commonlicense.Checker
		wantSecretCreated bool
		wantRequeue       bool
		wantErr           bool
	}{
		{
			name: "No policies exist - should create empty secret (enterprise enabled)",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "No policies exist - should create empty secret (enterprise disabled)",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: false},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Policy targets ES cluster in same namespace - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
		{
			name: "Policy targets ES cluster from operator namespace - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "global-policy",
						Namespace: "elastic-system",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
		{
			name: "Policy exists but does not target ES cluster - should create empty secret",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "other-app",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Policy in different namespace (not operator namespace) - should create empty secret",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app": "elasticsearch",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-ns-policy",
						Namespace: "other-namespace",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "elasticsearch",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: true,
			wantRequeue:       false,
			wantErr:           false,
		},
		{
			name: "Multiple policies, one targets ES - should NOT create empty secret but requeue",
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
					Labels: map[string]string{
						"app":  "elasticsearch",
						"team": "platform",
					},
				},
			},
			policies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "unrelated-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "other-app",
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "matching-policy",
						Namespace: "default",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"team": "platform",
							},
						},
					},
				},
			},
			licenseChecker:    commonlicense.MockLicenseChecker{EnterpriseEnabled: true},
			wantSecretCreated: false,
			wantRequeue:       true,
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with initial objects
			var initObjs []client.Object
			for i := range tt.policies {
				initObjs = append(initObjs, &tt.policies[i])
			}
			for i := range tt.existingSecrets {
				initObjs = append(initObjs, &tt.existingSecrets[i])
			}
			initObjs = append(initObjs, tt.es)

			c := k8s.NewFakeClient(initObjs...)

			requeue, err := maybeReconcileEmptyFileSettingsSecret(t.Context(), c, tt.licenseChecker, tt.es, operatorNamespace)

			// Check error expectation
			if tt.wantErr {
				assert.Error(t, err, "expected error at maybeReconcileEmptyFileSettingsSecret")
				return
			}
			assert.NoError(t, err, "expected no error at maybeReconcileEmptyFileSettingsSecret")
			assert.Equal(t, tt.wantRequeue, requeue, "expected requeue does not match")

			// Check if secret was created
			var secret corev1.Secret
			secretName := esv1.FileSettingsSecretName(tt.es.Name)
			secretErr := c.Get(t.Context(), types.NamespacedName{
				Name:      secretName,
				Namespace: tt.es.Namespace,
			}, &secret)

			if tt.wantSecretCreated {
				assert.NoError(t, secretErr, "expected no error at getting file-settings secret")
			} else {
				assert.True(t, apierrors.IsNotFound(secretErr), "expected IsNotFound error at getting file-settings secret")
			}
		})
	}
}
