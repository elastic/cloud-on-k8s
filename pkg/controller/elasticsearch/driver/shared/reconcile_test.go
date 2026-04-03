// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shared

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonlicense "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/fips"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
				t.Errorf("EsReachableConditionMessage() = %v, want %v", got, tt.want)
			}
		})
	}
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
			name: "Multiple policies, one targets ES, file-settings secret does not exist - should NOT create empty secret but requeue",
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
		{
			name: "Multiple policies, one targets ES, file-settings secret exists - should NOT requeue",
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
			existingSecrets: []corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esv1.FileSettingsSecretName("test-es"),
						Namespace: "default",
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
			wantSecretCreated: true,
			wantRequeue:       false,
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

func TestReconcileFIPSKeystoreSecretVersionGate(t *testing.T) {
	fipsConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &fipsConfig,
				},
			},
		},
	}

	tests := []struct {
		name         string
		esVersion    commonversion.Version
		wantSecret   bool
		wantSecretID string
	}{
		{
			name:         "below minimum version does not reconcile secret",
			esVersion:    commonversion.MinFor(9, 3, 0),
			wantSecret:   false,
			wantSecretID: esv1.FIPSKeystorePasswordSecret(es.Name),
		},
		{
			name:         "minimum version reconciles secret",
			esVersion:    esversion.FIPSKeystorePasswordMinVersion,
			wantSecret:   true,
			wantSecretID: esv1.FIPSKeystorePasswordSecret(es.Name),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(&es)

			secret, err := reconcileFIPSKeystoreSecret(context.Background(), c, es, tt.esVersion, metadata.Metadata{}, nodespec.PolicyConfig{})
			require.NoError(t, err)
			if tt.wantSecret {
				require.NotNil(t, secret)
				require.Equal(t, tt.wantSecretID, secret.Name)
			} else {
				require.Nil(t, secret)
			}

			var stored corev1.Secret
			err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: tt.wantSecretID}, &stored)
			if tt.wantSecret {
				require.NoError(t, err)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}

func TestReconcileFIPSKeystoreSecret_userKeystorePasswordOverride(t *testing.T) {
	fipsConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &fipsConfig,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Env: []corev1.EnvVar{
										{Name: essettings.KeystorePasswordEnvVar, Value: "user-supplied"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	c := k8s.NewFakeClient(&es)
	secret, err := reconcileFIPSKeystoreSecret(context.Background(), c, es, esversion.FIPSKeystorePasswordMinVersion, metadata.Metadata{}, nodespec.PolicyConfig{})
	require.NoError(t, err)
	require.Nil(t, secret)

	var stored corev1.Secret
	err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: esv1.FIPSKeystorePasswordSecret(es.Name)}, &stored)
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconcileFIPSKeystoreSecret_fipsDisabled_deletesSecret(t *testing.T) {
	nonFIPSConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &nonFIPSConfig,
				},
			},
		},
	}
	secretName := esv1.FIPSKeystorePasswordSecret(es.Name)
	existing := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			fips.KeystorePasswordKey: []byte("leftover-password"),
		},
	}

	c := k8s.NewFakeClient(&es, existing)
	secret, err := reconcileFIPSKeystoreSecret(context.Background(), c, es, esversion.FIPSKeystorePasswordMinVersion, metadata.Metadata{}, nodespec.PolicyConfig{})
	require.NoError(t, err)
	require.Nil(t, secret)

	err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: secretName}, &corev1.Secret{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestReconcileFIPSKeystoreSecret_fipsEnabledViaStackConfigPolicy(t *testing.T) {
	nonFIPSNodeConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &nonFIPSNodeConfig,
				},
			},
		},
	}
	policyCfg := commonsettings.MustCanonicalConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	policyConfig := nodespec.PolicyConfig{
		ElasticsearchConfig: policyCfg,
	}

	c := k8s.NewFakeClient(&es)
	secret, err := reconcileFIPSKeystoreSecret(context.Background(), c, es, esversion.FIPSKeystorePasswordMinVersion, metadata.Metadata{}, policyConfig)
	require.NoError(t, err)
	require.NotNil(t, secret)
	require.Equal(t, esv1.FIPSKeystorePasswordSecret(es.Name), secret.Name)
}
