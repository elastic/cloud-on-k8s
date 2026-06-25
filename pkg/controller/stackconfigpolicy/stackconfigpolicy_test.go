// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_getStackPolicyConfigForElasticsearch(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		operatorNamespace     string
		k8sObjects            []client.Object
		targetElasticsearch   *esv1.Elasticsearch
		stackConfigPolicies   []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy  policyv1alpha1.StackConfigPolicy
		expectedSecretSources []commonv1.NamespacedSecretSource
		expectedPolicyRefs    map[string]struct{}
		expectedMergeConflict bool
		wantErr               bool
	}{
		{
			name: "merges without overwrites",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 10,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "policy1-deprecated-secure-setting",
							},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"policy1.name": "policy1",
							}},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"type": "gcp",
								},
							}},
							SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"schedule": "0 1 2 3 4 ?",
								},
							}},
							SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"enabled": true,
								},
							}},
							SecurityRoles: &commonv1.Config{Data: map[string]any{
								"policy1_role": map[string]any{
									"cluster": []any{"monitor"},
									"indices": []any{
										map[string]any{
											"names":      []any{".monitoring-*"},
											"privileges": []any{"read"},
										},
									},
								},
							}},
							IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"phases": map[string]any{
										"delete": map[string]any{
											"actions": map[string]any{
												"delete": map[string]any{},
											},
										},
									},
								},
							}},
							IngestPipelines: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"description": "description",
								},
							}},
							IndexTemplates: policyv1alpha1.IndexTemplates{
								ComponentTemplates: &commonv1.Config{Data: map[string]any{
									"policy1": map[string]any{
										"template": map[string]any{},
									},
								}},
								ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
									"policy1": map[string]any{
										"priority": 500,
									},
								}},
							},
							Config: &commonv1.Config{Data: map[string]any{
								"node.roles": []any{"policy1"},
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "policy1-secure-setting",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy1-mount-path"},
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy2",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "policy2-deprecated-secure-setting",
							},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"policy2.name": "policy2",
							}},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy2": map[string]any{
									"type": "gcp",
								},
							}},
							SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy2": map[string]any{
									"schedule": "0 1 2 3 4 ?",
								},
							}},
							SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
								"policy2": map[string]any{
									"enabled": true,
								},
							}},
							SecurityRoles: &commonv1.Config{Data: map[string]any{
								"policy2_role": map[string]any{
									"cluster": []any{"manage"},
									"indices": []any{
										map[string]any{
											"names":      []any{"logs-*"},
											"privileges": []any{"write", "create_index"},
										},
									},
								},
							}},
							IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy2": map[string]any{
									"phases": map[string]any{
										"delete": map[string]any{
											"actions": map[string]any{
												"delete": map[string]any{},
											},
										},
									},
								},
							}},
							IngestPipelines: &commonv1.Config{Data: map[string]any{
								"policy2": map[string]any{
									"description": "description",
								},
							}},
							IndexTemplates: policyv1alpha1.IndexTemplates{
								ComponentTemplates: &commonv1.Config{Data: map[string]any{
									"policy2": map[string]any{
										"template": map[string]any{},
									},
								}},
								ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
									"policy2": map[string]any{
										"priority": 500,
									},
								}},
							},
							Config: &commonv1.Config{Data: map[string]any{
								"node.roles": []any{"policy2"},
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "policy2-secure-setting",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy2-mount-path"},
									},
								},
							},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: canonicaliseMap(t, map[string]any{
							"policy1.name": "policy1",
							"policy2.name": "policy2",
						})},
						SnapshotRepositories: &commonv1.Config{Data: map[string]any{
							"policy1": map[string]any{
								"type": "gcp",
							},
							"policy2": map[string]any{
								"type": "gcp",
							},
						}},
						SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
							"policy1": map[string]any{
								"schedule": "0 1 2 3 4 ?",
							},
							"policy2": map[string]any{
								"schedule": "0 1 2 3 4 ?",
							},
						}},
						SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
							"policy1": map[string]any{
								"enabled": true,
							},
							"policy2": map[string]any{
								"enabled": true,
							},
						}},
						SecurityRoles: &commonv1.Config{Data: map[string]any{
							"policy1_role": map[string]any{
								"cluster": []any{"monitor"},
								"indices": []any{
									map[string]any{
										"names":      []any{".monitoring-*"},
										"privileges": []any{"read"},
									},
								},
							},
							"policy2_role": map[string]any{
								"cluster": []any{"manage"},
								"indices": []any{
									map[string]any{
										"names":      []any{"logs-*"},
										"privileges": []any{"write", "create_index"},
									},
								},
							},
						}},
						IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
							"policy1": map[string]any{
								"phases": map[string]any{
									"delete": map[string]any{
										"actions": map[string]any{
											"delete": map[string]any{},
										},
									},
								},
							},
							"policy2": map[string]any{
								"phases": map[string]any{
									"delete": map[string]any{
										"actions": map[string]any{
											"delete": map[string]any{},
										},
									},
								},
							},
						}},
						IngestPipelines: &commonv1.Config{Data: map[string]any{
							"policy1": map[string]any{
								"description": "description",
							},
							"policy2": map[string]any{
								"description": "description",
							},
						}},
						IndexTemplates: policyv1alpha1.IndexTemplates{
							ComponentTemplates: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"template": map[string]any{},
								},
								"policy2": map[string]any{
									"template": map[string]any{},
								},
							}},
							ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
								"policy1": map[string]any{
									"priority": float64(500),
								},
								"policy2": map[string]any{
									"priority": float64(500),
								},
							}},
						},
						Config: &commonv1.Config{Data: canonicaliseMap(t, map[string]any{
							"node.roles": []any{"policy2", "policy1"},
						})},
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
							{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
						},
					},
				},
			},
			expectedSecretSources: []commonv1.NamespacedSecretSource{
				{
					SecretName: "policy1-deprecated-secure-setting",
					Namespace:  "test",
				},
				{
					SecretName: "policy1-secure-setting",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy1-mount-path"},
					},
				},
				{
					SecretName: "policy2-deprecated-secure-setting",
					Namespace:  "test",
				},
				{
					SecretName: "policy2-secure-setting",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy2-mount-path"},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/policy1": {},
				"test/policy2": {},
			},
		}, {
			name: "merges with overwrites",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 10,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "policy1-deprecated-secure-setting",
							},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"policy.name": "policy1",
							}},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"type": "gcp",
								},
							}},
							SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"schedule": "0 1 2 3 4 ?",
								},
							}},
							SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"enabled": true,
								},
							}},
							SecurityRoles: &commonv1.Config{Data: map[string]any{
								"custom_role": map[string]any{
									"cluster": []any{"monitor"},
									"indices": []any{
										map[string]any{
											"names":      []any{"metrics-*"},
											"privileges": []any{"read"},
										},
									},
								},
							}},
							IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"phases": map[string]any{
										"delete": map[string]any{
											"actions": map[string]any{
												"delete": map[string]any{},
											},
										},
									},
								},
							}},
							IngestPipelines: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"description": "policy1",
								},
							}},
							IndexTemplates: policyv1alpha1.IndexTemplates{
								ComponentTemplates: &commonv1.Config{Data: map[string]any{
									"policy": map[string]any{
										"template": map[string]any{},
									},
								}},
								ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
									"policy": map[string]any{
										"priority": 500,
									},
								}},
							},
							Config: &commonv1.Config{Data: map[string]any{
								"node.store.allow_mmap": false,
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "policy1-secure-setting",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy1-mount-path"},
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy2",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "policy2-deprecated-secure-setting",
							},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"policy.name": "policy2",
							}},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"type": "fs",
								},
							}},
							SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"schedule": "5 ?",
								},
							}},
							SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"enabled": false,
								},
							}},
							SecurityRoles: &commonv1.Config{Data: map[string]any{
								"custom_role": map[string]any{
									"cluster": []any{"manage_security"},
									"indices": []any{
										map[string]any{
											"names":      []any{"*"},
											"privileges": []any{"all"},
										},
									},
								},
							}},
							IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"phases": map[string]any{
										"delete": map[string]any{
											"actions": map[string]any{
												"delete": []any{"*"},
											},
										},
									},
								},
							}},
							IngestPipelines: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"description": "policy2",
								},
							}},
							IndexTemplates: policyv1alpha1.IndexTemplates{
								ComponentTemplates: &commonv1.Config{Data: map[string]any{
									"policy": map[string]any{
										"template": map[string]any{
											"properties": map[string]any{},
										},
									},
								}},
								ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
									"policy": map[string]any{
										"priority": 300,
									},
								}},
							},
							Config: &commonv1.Config{Data: map[string]any{
								"node.store.allow_mmap": true,
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "policy2-secure-setting",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy2-mount-path"},
									},
								},
							},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: canonicaliseMap(t, map[string]any{
							"policy.name": "policy1",
						})},
						SnapshotRepositories: &commonv1.Config{Data: map[string]any{
							"policy": map[string]any{
								"type": "gcp",
							},
						}},
						SnapshotLifecyclePolicies: &commonv1.Config{Data: map[string]any{
							"policy": map[string]any{
								"schedule": "0 1 2 3 4 ?",
							},
						}},
						SecurityRoleMappings: &commonv1.Config{Data: map[string]any{
							"policy": map[string]any{
								"enabled": true,
							},
						}},
						SecurityRoles: &commonv1.Config{Data: map[string]any{
							"custom_role": map[string]any{
								"cluster": []any{"monitor"},
								"indices": []any{
									map[string]any{
										"names":      []any{"metrics-*"},
										"privileges": []any{"read"},
									},
								},
							},
						}},
						IndexLifecyclePolicies: &commonv1.Config{Data: map[string]any{
							"policy": map[string]any{
								"phases": map[string]any{
									"delete": map[string]any{
										"actions": map[string]any{
											"delete": map[string]any{},
										},
									},
								},
							},
						}},
						IngestPipelines: &commonv1.Config{Data: map[string]any{
							"policy": map[string]any{
								"description": "policy1",
							},
						}},
						IndexTemplates: policyv1alpha1.IndexTemplates{
							ComponentTemplates: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"template": map[string]any{},
								},
							}},
							ComposableIndexTemplates: &commonv1.Config{Data: map[string]any{
								"policy": map[string]any{
									"priority": float64(500),
								},
							}},
						},
						Config: &commonv1.Config{Data: canonicaliseMap(t, map[string]any{
							"node.store.allow_mmap": false,
						})},
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
							{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
						},
					},
				},
			},
			expectedSecretSources: []commonv1.NamespacedSecretSource{
				{
					SecretName: "policy1-deprecated-secure-setting",
					Namespace:  "test",
				},
				{
					SecretName: "policy1-secure-setting",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy1-mount-path"},
					},
				},
				{
					SecretName: "policy2-deprecated-secure-setting",
					Namespace:  "test",
				},
				{
					SecretName: "policy2-secure-setting",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy2-mount-path"},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/policy1": {},
				"test/policy2": {},
			},
		}, {
			name: "no changes for single policy",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "policy1-deprecated-secure-setting-2",
							},
							{
								SecretName: "policy1-deprecated-secure-setting-1",
							},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"policy.name": "policy1",
							}},
							Config: &commonv1.Config{Data: map[string]any{
								"node.store.allow_mmap": false,
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
								{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
							},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "policy1-secure-setting-2",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy1-mount-path"},
									},
								},
								{
									SecretName: "policy1-secure-setting-1",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/policy1-mount-path"},
									},
								},
							},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: map[string]any{
							"policy.name": "policy1",
						}},
						Config: &commonv1.Config{Data: map[string]any{
							"node.store.allow_mmap": false,
						}},
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "policy2-secret-mount", MountPath: "/policy2-mount-path"},
							{SecretName: "policy1-secret-mount", MountPath: "/policy1-mount-path"},
						},
					},
				},
			},
			expectedSecretSources: []commonv1.NamespacedSecretSource{
				{
					SecretName: "policy1-deprecated-secure-setting-2",
					Namespace:  "test",
				},
				{
					SecretName: "policy1-deprecated-secure-setting-1",
					Namespace:  "test",
				},
				{
					SecretName: "policy1-secure-setting-2",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy1-mount-path"},
					},
				},
				{
					SecretName: "policy1-secure-setting-1",
					Namespace:  "test",
					Entries: []commonv1.KeyToPath{
						{Key: "test", Path: "/policy1-mount-path"},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/policy1": {},
			},
		},
		{
			name: "detects policies weight conflicts",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy with unique weight - should be merged
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy1",
							}},
						},
					},
				},
				// Two policies with the same weight - should conflict and be skipped
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy2-conflict",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 5,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"conflict": "policy2",
							}},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy3-conflict",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 5,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"conflict": "policy3",
							}},
						},
					},
				},
			},
			expectedMergeConflict: true,
		},
		{
			name: "detects conflicts when same secret defined in multiple policies",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy 1 with lower weight - attempts to define the same secret as Policy 2, should conflict
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy1",
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "shared-secret", MountPath: "/mnt/policy1"},
							},
						},
					},
				},
				// Policy 2 with higher weight - should be merged first
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy2",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 5,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy2",
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "shared-secret", MountPath: "/mnt/policy2"},
							},
						},
					},
				},
			},
			expectedMergeConflict: true,
		},
		{
			name: "detects conflicts when same mount path defined in multiple policies",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy 1 with lower weight - attempts to use the same mount path, should conflict
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy1",
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret1", MountPath: "/mnt/shared"},
							},
						},
					},
				},
				// Policy 2 with higher weight - should be merged first
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy2",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 5,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy2",
							}},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret2", MountPath: "/mnt/shared"},
							},
						},
					},
				},
			},
			expectedMergeConflict: true,
		},
		{
			name:              "elasticsearch different namespace",
			operatorNamespace: "operator-namespace",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "es-namespace",
					Labels: map[string]string{
						"env": "production",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy in wrong namespace - should not match
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "wrong-namespace",
						Name:            "policy-wrong-ns",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "production"},
						},
						Weight: 1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"should-not": "be-included",
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{},
				},
			},
			expectedPolicyRefs: map[string]struct{}{},
		},
		{
			name: "elasticsearch non-matching labels",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "test",
					Labels: map[string]string{
						"env":  "production",
						"team": "platform",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy with non-matching label selector - should not match
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy-wrong-labels",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"env": "development", // doesn't match ES labels
							},
						},
						Weight: 1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"should-not": "be-included",
							}},
						},
					},
				},
				// Policy with partially matching labels - should not match
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy-partial-match",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"env":     "production",
								"team":    "platform",
								"missing": "label", // this label doesn't exist on ES
							},
						},
						Weight: 2,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"also-should-not": "be-included",
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{},
				},
			},
			expectedPolicyRefs: map[string]struct{}{},
		},
		{
			name: "variablesFrom: substitutes variables from ConfigMap and Secret",
			k8sObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "repo-vars", Namespace: "test"},
					Data:       map[string]string{"BUCKET_NAME": "my-bucket", "REGION": "eu-west-1"},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "repo-creds", Namespace: "test"},
					Data:       map[string][]byte{"ACCESS_KEY": []byte("AKIAIOSFODNN7")},
				},
			},
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", Labels: map[string]string{"test": "test"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "vars-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"test": "test"}},
						Weight:           10,
						VariablesFrom: []policyv1alpha1.VariableSource{
							{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "repo-vars"},
							{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "repo-creds"},
						},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"my-repo": map[string]any{
									"type": "s3",
									"settings": map[string]any{
										"bucket":     "${BUCKET_NAME}",
										"region":     "${REGION}",
										"access_key": "${ACCESS_KEY}",
									},
								},
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						SnapshotRepositories: &commonv1.Config{Data: map[string]any{
							"my-repo": map[string]any{
								"type": "s3",
								"settings": map[string]any{
									"bucket":     "my-bucket",
									"region":     "eu-west-1",
									"access_key": "AKIAIOSFODNN7",
								},
							},
						}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{"test/vars-policy": {}},
		},
		{
			name: "variablesFrom: no VariablesFrom is a no-op",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", Labels: map[string]string{"test": "test"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "no-vars-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"test": "test"}},
						Weight:           10,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "value"}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "value"}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{"test/no-vars-policy": {}},
		},
		{
			name: "variablesFrom: missing non-optional source returns error",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test", Labels: map[string]string{"test": "test"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "missing-vars-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"test": "test"}},
						Weight:           10,
						VariablesFrom:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "missing-cm"}},
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}},
						},
					},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			esConfigPolicy, err := getConfigPolicyForElasticsearch(context.Background(), k8s.NewFakeClient(tc.k8sObjects...), tc.targetElasticsearch, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			// Check for expected conflict errors
			if tc.expectedMergeConflict {
				assert.ErrorIs(t, err, errMergeConflict, "getConfigPolicyForElasticsearch should return an error")
				return
			}
			assert.NoError(t, err, "getConfigPolicyForElasticsearch should not return an error")

			// Compare secret sources if expected
			if tc.expectedSecretSources != nil {
				assert.EqualValues(t, tc.expectedSecretSources, esConfigPolicy.SecretSources)
			}

			assert.EqualValues(t, tc.expectedConfigPolicy.Spec.Elasticsearch, esConfigPolicy.Spec)

			// Compare policy references by building a map from the actual refs
			actualPolicyRefs := make(map[string]struct{})
			for _, policy := range esConfigPolicy.PolicyRefs {
				nsn := types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}
				actualPolicyRefs[nsn.String()] = struct{}{}
			}
			assert.EqualValues(t, tc.expectedPolicyRefs, actualPolicyRefs)
		})
	}
}

func Test_getPolicyConfigForKibana(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		operatorNamespace     string
		k8sObjects            []client.Object
		targetKibana          *kbv1.Kibana
		stackConfigPolicies   []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy  policyv1alpha1.StackConfigPolicy
		expectedSecretSources []commonv1.NamespacedSecretSource
		expectedPolicyRefs    map[string]struct{}
		expectedMergeConflict bool
		wantErr               bool
	}{
		{
			name: "merges Kibana configs without overwrites",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"app": "kibana",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-policy1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "kibana"},
						},
						Weight: 10,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.canvas.enabled": true,
								"logging.root.level":   "info",
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "kb-secret1",
									Entries:    []commonv1.KeyToPath{{Key: "key1", Path: "path1"}},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-policy2",
						ResourceVersion: "2",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "kibana"},
						},
						Weight: 20,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.reporting.enabled": false,
								"server.maxPayload":       float64(2097152),
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "kb-secret2",
									Entries:    []commonv1.KeyToPath{{Key: "key2", Path: "path2"}},
								},
							},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{
						Config: &commonv1.Config{Data: map[string]any{
							"xpack": map[string]any{
								"canvas": map[string]any{
									"enabled": true,
								},
								"reporting": map[string]any{
									"enabled": false,
								},
							},
							"logging": map[string]any{
								"root": map[string]any{
									"level": "info",
								},
							},
							"server": map[string]any{
								"maxPayload": float64(2097152),
							},
						}},
					},
				},
			},
			expectedSecretSources: []commonv1.NamespacedSecretSource{
				{
					SecretName: "kb-secret1",
					Namespace:  "test",
					Entries:    []commonv1.KeyToPath{{Key: "key1", Path: "path1"}},
				},
				{
					SecretName: "kb-secret2",
					Namespace:  "test",
					Entries:    []commonv1.KeyToPath{{Key: "key2", Path: "path2"}},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/kb-policy1": {},
				"test/kb-policy2": {},
			},
		},
		{
			name: "merges Kibana configs with overwrites - higher weight wins",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-low-priority",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Weight: 10,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"logging.root.level": "info",
								"server.port":        5601,
							}},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-high-priority",
						ResourceVersion: "2",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "prod"},
						},
						Weight: 2,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"logging.root.level": "debug", // Override from policy with weight 10
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{
						Config: &commonv1.Config{Data: map[string]any{
							"logging": map[string]any{
								"root": map[string]any{
									"level": "info",
								},
							},
							"server": map[string]any{
								"port": uint64(5601),
							},
						}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/kb-low-priority":  {},
				"test/kb-high-priority": {},
			},
		},
		{
			name: "Kibana policies with same weight cause conflict",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"env": "staging",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-conflict-1",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "staging"},
						},
						Weight: 15,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.canvas.enabled": true,
							}},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-conflict-2",
						ResourceVersion: "2",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"env": "staging"},
						},
						Weight: 15, // Same weight as kb-conflict-1
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.reporting.enabled": false,
							}},
						},
					},
				},
			},
			expectedMergeConflict: true,
		},
		{
			name: "Kibana policy doesn't match due to namespace",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "prod",
					Labels: map[string]string{
						"app": "kibana",
					},
				},
			},
			operatorNamespace: "elastic-system",
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "dev",
						Name:            "kb-policy-wrong-ns",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "kibana"},
						},
						Weight: 10,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.canvas.enabled": true,
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{},
				},
			},
			expectedPolicyRefs: map[string]struct{}{},
		},
		{
			name: "Kibana policy doesn't match due to labels",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"app": "kibana",
						"env": "prod",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-policy-wrong-labels",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": "kibana",
								"env": "dev", // Different value
							},
						},
						Weight: 10,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: canonicaliseMap(t, map[string]any{
								"xpack.canvas.enabled": true,
							})},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{},
				},
			},
			expectedPolicyRefs: map[string]struct{}{},
		},
		{
			name: "no changes for single policy",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"app": "kibana",
					},
				},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "kb-single-policy",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "kibana"},
						},
						Weight: 10,
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{
								"xpack.canvas.enabled": true,
								"logging.root.level":   "info",
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{
						Config: &commonv1.Config{Data: map[string]any{
							"xpack.canvas.enabled": true,
							"logging.root.level":   "info",
						}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/kb-single-policy": {},
			},
		},
		{
			name: "variablesFrom: substitutes variables from ConfigMap",
			k8sObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "kb-vars", Namespace: "test"},
					Data:       map[string]string{"MAX_PAYLOAD": "2097152"},
				},
			},
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Name: "test-kb", Namespace: "test", Labels: map[string]string{"app": "kibana"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "kb-vars-cm-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kibana"}},
						Weight:           10,
						VariablesFrom:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "kb-vars"}},
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{"server.maxPayload": "${MAX_PAYLOAD}"}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{
						Config: &commonv1.Config{Data: map[string]any{"server.maxPayload": "2097152"}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{"test/kb-vars-cm-policy": {}},
		},
		{
			name: "variablesFrom: substitutes variables from Secret",
			k8sObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "kb-creds", Namespace: "test"},
					Data:       map[string][]byte{"LOG_LEVEL": []byte("debug")},
				},
			},
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Name: "test-kb", Namespace: "test", Labels: map[string]string{"app": "kibana"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "kb-vars-secret-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kibana"}},
						Weight:           10,
						VariablesFrom:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindSecret, Name: "kb-creds"}},
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{"logging.root.level": "${LOG_LEVEL}"}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Kibana: policyv1alpha1.KibanaConfigPolicySpec{
						Config: &commonv1.Config{Data: map[string]any{"logging.root.level": "debug"}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{"test/kb-vars-secret-policy": {}},
		},
		{
			name: "variablesFrom: missing non-optional source returns error",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{Name: "test-kb", Namespace: "test", Labels: map[string]string{"app": "kibana"}},
			},
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test", Name: "kb-missing-vars-policy", ResourceVersion: "1"},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "kibana"}},
						Weight:           10,
						VariablesFrom:    []policyv1alpha1.VariableSource{{Kind: policyv1alpha1.VariableSourceKindConfigMap, Name: "missing-cm"}},
						Kibana: policyv1alpha1.KibanaConfigPolicySpec{
							Config: &commonv1.Config{Data: map[string]any{"key": "${VAR}"}},
						},
					},
				},
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kbPolicyConfig, err := getConfigPolicyForKibana(context.Background(), k8s.NewFakeClient(tc.k8sObjects...), tc.targetKibana, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			// Verify conflict errors
			if tc.expectedMergeConflict {
				assert.ErrorIs(t, err, errMergeConflict)
				return
			}
			assert.NoError(t, err)

			// Compare secret sources if expected
			if tc.expectedSecretSources != nil {
				assert.EqualValues(t, tc.expectedSecretSources, kbPolicyConfig.SecretSources)
			}

			assert.EqualValues(t, tc.expectedConfigPolicy.Spec.Kibana, kbPolicyConfig.Spec)

			// Compare policy references
			actualPolicyRefs := make(map[string]struct{})
			for _, policy := range kbPolicyConfig.PolicyRefs {
				nsn := types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}
				actualPolicyRefs[nsn.String()] = struct{}{}
			}
			assert.EqualValues(t, tc.expectedPolicyRefs, actualPolicyRefs)
		})
	}
}

func canonicaliseMap(t *testing.T, src map[string]any) map[string]any {
	t.Helper()

	dstCanonicalConfig, err := settings.NewCanonicalConfigFrom(src)
	require.NoError(t, err, "failed to canonicalise map")

	var canonicalisedMap map[string]any
	err = dstCanonicalConfig.Unpack(&canonicalisedMap)
	require.NoError(t, err, "failed to unpack")
	return canonicalisedMap
}
