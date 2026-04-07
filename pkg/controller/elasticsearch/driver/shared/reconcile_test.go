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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password/fixtures"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
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

func TestReconcileManagedKeystorePasswordSecret(t *testing.T) {
	fipsEnabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	fipsDisabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})

	esMeta := metav1.ObjectMeta{Namespace: "ns", Name: "es"}
	keystorePasswordSecretName := esv1.KeystorePasswordSecret(esMeta.Name)

	esFIPSNodeSetOnly := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{Name: "default", Config: &fipsEnabledConfig},
			},
		},
	}
	esFIPSWithUserKeystorePassword := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &fipsEnabledConfig,
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
	esFIPSDisabled := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{Name: "default", Config: &fipsDisabledConfig},
			},
		},
	}
	existingKeystorePasswordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esMeta.Namespace,
			Name:      keystorePasswordSecretName,
		},
		Data: map[string][]byte{
			keystorepassword.KeystorePasswordKey: []byte("leftover-password"),
		},
	}
	policyFIPSEnabled := nodespec.PolicyConfig{
		ElasticsearchConfig: commonsettings.MustCanonicalConfig(map[string]any{
			"xpack.security.fips_mode.enabled": true,
		}),
	}

	tests := []struct {
		name               string
		es                 esv1.Elasticsearch
		extraInit          []client.Object
		policyConfig       nodespec.PolicyConfig
		esVersion          commonversion.Version
		wantReturnedSecret bool
		wantSecretInAPI    bool
	}{
		{
			name:               "below minimum version does not reconcile secret",
			es:                 esFIPSNodeSetOnly,
			esVersion:          commonversion.MinFor(9, 3, 0),
			wantReturnedSecret: false,
			wantSecretInAPI:    false,
		},
		{
			name:               "minimum version reconciles secret",
			es:                 esFIPSNodeSetOnly,
			esVersion:          esversion.FIPSKeystorePasswordMinVersion,
			wantReturnedSecret: true,
			wantSecretInAPI:    true,
		},
		{
			name:               "user-provided keystore password env skips operator secret",
			es:                 esFIPSWithUserKeystorePassword,
			esVersion:          esversion.FIPSKeystorePasswordMinVersion,
			wantReturnedSecret: false,
			wantSecretInAPI:    false,
		},
		{
			name:               "FIPS disabled does not change helper behavior",
			es:                 esFIPSDisabled,
			extraInit:          []client.Object{existingKeystorePasswordSecret},
			esVersion:          esversion.FIPSKeystorePasswordMinVersion,
			wantReturnedSecret: true,
			wantSecretInAPI:    true,
		},
		{
			name:               "FIPS enabled only via StackConfigPolicy reconciles secret",
			es:                 esFIPSDisabled,
			policyConfig:       policyFIPSEnabled,
			esVersion:          esversion.FIPSKeystorePasswordMinVersion,
			wantReturnedSecret: true,
			wantSecretInAPI:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.es
			initObjs := append([]client.Object{&es}, tt.extraInit...)
			c := k8s.NewFakeClient(initObjs...)

			secret, err := reconcileManagedKeystorePasswordSecret(context.Background(), c, es, tt.esVersion, fixtures.MustTestRandomGenerator(24), metadata.Metadata{})
			require.NoError(t, err)
			if tt.wantReturnedSecret {
				require.NotNil(t, secret)
				require.Equal(t, keystorePasswordSecretName, secret.Name)
			} else {
				require.Nil(t, secret)
			}

			var stored corev1.Secret
			err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: keystorePasswordSecretName}, &stored)
			if tt.wantSecretInAPI {
				require.NoError(t, err)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}
