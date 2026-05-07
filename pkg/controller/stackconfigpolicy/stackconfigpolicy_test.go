// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
)

func Test_getStackPolicyConfigForElasticsearch(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		operatorNamespace     string
		targetElasticsearch   *esv1.Elasticsearch
		stackConfigPolicies   []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy  policyv1alpha1.StackConfigPolicy
		expectedSecretSources []commonv1.NamespacedSecretSource
		expectedPolicyRefs    map[string]struct{}
		expectedMergeConflict bool
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			esConfigPolicy, err := getConfigPolicyForElasticsearch(tc.targetElasticsearch, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
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
		targetKibana          *kbv1.Kibana
		stackConfigPolicies   []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy  policyv1alpha1.StackConfigPolicy
		expectedSecretSources []commonv1.NamespacedSecretSource
		expectedPolicyRefs    map[string]struct{}
		expectedMergeConflict bool
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			kbPolicyConfig, err := getConfigPolicyForKibana(tc.targetKibana, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
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
