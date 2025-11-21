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
)

func Test_getStackPolicyConfigForElasticsearch(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		policyNamespace        string
		operatorNamespace      string
		targetElasticsearch    *esv1.Elasticsearch
		stackConfigPolicies    []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy   policyv1alpha1.StackConfigPolicy
		expectedPolicyRefs     map[string]struct{}
		expectedConflictErrors map[types.NamespacedName]bool // map of policy names that should have conflict errors
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
			policyNamespace: "test",
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
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"test1.name": "policy1",
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "test",
									Entries: []commonv1.KeyToPath{
										{Key: "test", Path: "/test-policy1"},
									},
								},
							},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret-policy1", MountPath: "/secret-policy1"},
							},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy-backups.type": "fs",
								"policy-backups": map[string]any{
									"settings.location": "/backups",
								},
							}},
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
						Weight: -1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"test2.name": "policy2",
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "test",
									Entries: []commonv1.KeyToPath{
										{Key: "test1", Path: "/test1-policy2"},
										{Key: "test2", Path: "/test2-policy2"},
									},
								},
							},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret-policy2", MountPath: "/secret-policy2"},
							},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy-2-backups.type": "s3",
								"policy-2-backups.settings": map[string]any{
									"bucket": "policy-2-backups",
									"region": "us-west-2",
								},
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: map[string]any{
							"test1.name": "policy1",
							"test2.name": "policy2",
						}},
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "test",
								Entries: []commonv1.KeyToPath{
									{Key: "test", Path: "/test-policy1"},
									{Key: "test1", Path: "/test1-policy2"},
									{Key: "test2", Path: "/test2-policy2"},
								},
							},
						},
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "secret-policy1", MountPath: "/secret-policy1"},
							{SecretName: "secret-policy2", MountPath: "/secret-policy2"},
						},
						SnapshotRepositories: &commonv1.Config{Data: map[string]any{
							"policy-2-backups.type": "s3",
							"policy-2-backups.settings": map[string]any{
								"bucket": "policy-2-backups",
								"region": "us-west-2",
							},
							"policy-backups.type": "fs",
							"policy-backups": map[string]any{
								"settings.location": "/backups",
							},
						}},
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
			policyNamespace: "test",
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
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"test.name": "policy1",
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "test",
									Entries: []commonv1.KeyToPath{
										{Key: "test1", Path: "/test1-policy1"},
										{Key: "test2", Path: "/test2-policy1"},
									},
								},
							},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret-policy-1", MountPath: "/secret-policy1"},
							},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy-2-backups.type": "fs",
								"policy-2-backups.settings": map[string]any{
									"location": "/tmp/location",
								},
							}},
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
						Weight: -1,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"test.name": "policy2",
							}},
							SecureSettings: []commonv1.SecretSource{
								{
									SecretName: "test",
									Entries: []commonv1.KeyToPath{
										{Key: "test1", Path: "/test1-policy2"},
										{Key: "test2", Path: "/test2-policy2"},
									},
								},
							},
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "secret-policy-2", MountPath: "/secret-policy2"},
							},
							SnapshotRepositories: &commonv1.Config{Data: map[string]any{
								"policy-2-backups.type": "s3",
								"policy-2-backups.settings": map[string]any{
									"bucket": "policy-2-backups",
									"region": "us-west-2",
								},
							}},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						ClusterSettings: &commonv1.Config{Data: map[string]any{
							"test.name": "policy2",
						}},
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "test",
								Entries: []commonv1.KeyToPath{
									{Key: "test1", Path: "/test1-policy2"},
									{Key: "test2", Path: "/test2-policy2"},
								},
							},
						},
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "secret-policy-1", MountPath: "/secret-policy1"},
							{SecretName: "secret-policy-2", MountPath: "/secret-policy2"},
						},
						SnapshotRepositories: &commonv1.Config{Data: map[string]any{
							"policy-2-backups.type": "s3",
							"policy-2-backups.settings": map[string]any{
								"bucket": "policy-2-backups",
								"region": "us-west-2",
							},
						}},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/policy1": {},
				"test/policy2": {},
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
			policyNamespace: "test",
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
				// Another policy with unique weight - should be merged
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "test",
						Name:            "policy4",
						ResourceVersion: "1",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						ResourceSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"test": "test"},
						},
						Weight: 10,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							ClusterSettings: &commonv1.Config{Data: map[string]any{
								"from": "policy4",
							}},
						},
					},
				},
			},
			expectedConflictErrors: map[types.NamespacedName]bool{
				{Namespace: "test", Name: "policy2-conflict"}: true,
				{Namespace: "test", Name: "policy3-conflict"}: true,
			},
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
			policyNamespace: "test",
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy 1 with lower weight - should be merged first
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
				// Policy 2 with higher weight - attempts to define the same secret, should conflict
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
			expectedConflictErrors: map[types.NamespacedName]bool{
				{Namespace: "test", Name: "policy2"}: true,
				{Namespace: "test", Name: "policy1"}: true,
			},
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
			policyNamespace: "test",
			stackConfigPolicies: []policyv1alpha1.StackConfigPolicy{
				// Policy 1 with lower weight - should be merged first
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
				// Policy 2 with higher weight - attempts to use the same mount path, should conflict
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
			expectedConflictErrors: map[types.NamespacedName]bool{
				{Namespace: "test", Name: "policy2"}: true,
				{Namespace: "test", Name: "policy1"}: true,
			},
		},
		{
			name: "successfully merges when different secrets use different mount paths",
			targetElasticsearch: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test",
					Labels: map[string]string{
						"test": "test",
					},
				},
			},
			policyNamespace: "test",
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
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "db-creds", MountPath: "/etc/db"},
								{SecretName: "api-keys", MountPath: "/etc/api"},
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
						Weight: 5,
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							SecretMounts: []policyv1alpha1.SecretMount{
								{SecretName: "tls-cert", MountPath: "/etc/tls"},
								{SecretName: "backup-creds", MountPath: "/etc/backup"},
							},
						},
					},
				},
			},
			expectedConfigPolicy: policyv1alpha1.StackConfigPolicy{
				Spec: policyv1alpha1.StackConfigPolicySpec{
					Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
						SecretMounts: []policyv1alpha1.SecretMount{
							{SecretName: "api-keys", MountPath: "/etc/api"},
							{SecretName: "backup-creds", MountPath: "/etc/backup"},
							{SecretName: "db-creds", MountPath: "/etc/db"},
							{SecretName: "tls-cert", MountPath: "/etc/tls"},
						},
					},
				},
			},
			expectedPolicyRefs: map[string]struct{}{
				"test/policy1": {},
				"test/policy2": {},
			},
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
			policyNamespace: "test",
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
			policyNamespace: "test",
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
			esStackConfig, err := getPolicyConfigForElasticsearch(tc.targetElasticsearch, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
			// Check for expected conflict errors
			if tc.expectedConflictErrors != nil {
				assert.ErrorIs(t, err, errMergeConflict, "getPolicyConfigForElasticsearch should return an error")
				assert.NotNil(t, esStackConfig.PoliciesWithConflictErrors, "should have conflict errors")
				assert.Len(t, esStackConfig.PoliciesWithConflictErrors, len(tc.expectedConflictErrors), "should have expected number of conflict errors")

				for policyNsn, shouldHaveError := range tc.expectedConflictErrors {
					if shouldHaveError {
						assert.Containsf(t, esStackConfig.PoliciesWithConflictErrors, policyNsn, "Expected conflict error for policy %s but found none", policyNsn)
					} else {
						assert.NotContainsf(t, esStackConfig.PoliciesWithConflictErrors, policyNsn, "Not expected conflict error for policy %s but found none", policyNsn)
					}
				}
				return
			}
			assert.NoError(t, err, "getPolicyConfigForElasticsearch should not return an error")
			assert.Empty(t, esStackConfig.PoliciesWithConflictErrors, "should not have any conflict errors")

			// we self-merge the expected config just to canonicalise it
			expectedConfigPolicyCopy := tc.expectedConfigPolicy.Spec.Elasticsearch.DeepCopy()
			expectedConfigPolicyCopy.SecretMounts = nil
			err = mergeElasticsearchConfig(&tc.expectedConfigPolicy.Spec.Elasticsearch, *expectedConfigPolicyCopy)
			require.NoError(t, err, "canonicalise expected config should not return an error")

			// Compare the merged Elasticsearch configuration
			assert.EqualValues(t, tc.expectedConfigPolicy.Spec.Elasticsearch, esStackConfig.Spec)

			// Compare policy references by building a map from the actual refs
			actualPolicyRefs := make(map[string]struct{})
			for _, policy := range esStackConfig.PoliciesRefs {
				nsn := types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}
				actualPolicyRefs[nsn.String()] = struct{}{}
			}
			assert.EqualValues(t, tc.expectedPolicyRefs, actualPolicyRefs)
		})
	}
}

func Test_getPolicyConfigForKibana(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		policyNamespace        string
		operatorNamespace      string
		targetKibana           *kbv1.Kibana
		stackConfigPolicies    []policyv1alpha1.StackConfigPolicy
		expectedConfigPolicy   policyv1alpha1.StackConfigPolicy
		expectedPolicyRefs     map[string]struct{}
		expectedConflictErrors map[types.NamespacedName]bool
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
			policyNamespace: "test",
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
						SecureSettings: []commonv1.SecretSource{
							{
								SecretName: "kb-secret1",
								Entries:    []commonv1.KeyToPath{{Key: "key1", Path: "path1"}},
							},
							{
								SecretName: "kb-secret2",
								Entries:    []commonv1.KeyToPath{{Key: "key2", Path: "path2"}},
							},
						},
					},
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
			policyNamespace: "test",
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
								"server.port":        uint64(5601),
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
						Weight: 20,
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
			policyNamespace: "test",
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
			expectedConflictErrors: map[types.NamespacedName]bool{
				{Namespace: "test", Name: "kb-conflict-1"}: true,
				{Namespace: "test", Name: "kb-conflict-2"}: true,
			},
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
			policyNamespace:   "dev",
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
			policyNamespace: "test",
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
			name: "Single Kibana policy - no merging optimization",
			targetKibana: &kbv1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test",
					Labels: map[string]string{
						"app": "kibana",
					},
				},
			},
			policyNamespace: "test",
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
			kbPolicyConfig, err := getPolicyConfigForKibana(tc.targetKibana, tc.stackConfigPolicies, operator.Parameters{
				OperatorNamespace: tc.operatorNamespace,
			})
			// Verify conflict errors
			if tc.expectedConflictErrors != nil {
				assert.ErrorIs(t, err, errMergeConflict)
				assert.NotNil(t, kbPolicyConfig.PoliciesWithConflictErrors, "Expected conflict errors but got none")
				for policyNsn, shouldHaveError := range tc.expectedConflictErrors {
					if shouldHaveError {
						assert.Containsf(t, kbPolicyConfig.PoliciesWithConflictErrors, policyNsn, "Expected conflict error for policy %s but found none", policyNsn)
					} else {
						assert.NotContainsf(t, kbPolicyConfig.PoliciesWithConflictErrors, policyNsn, "Not expected conflict error for policy %s but found none", policyNsn)
					}
				}
				return
			}
			assert.NoError(t, err)
			assert.Empty(t, kbPolicyConfig.PoliciesWithConflictErrors, "Expected no conflict errors")

			// Compare the merged Kibana configuration
			assert.EqualValues(t, tc.expectedConfigPolicy.Spec.Kibana, kbPolicyConfig.Spec)

			// Compare policy references
			actualPolicyRefs := make(map[string]struct{})
			for _, policy := range kbPolicyConfig.PoliciesRefs {
				nsn := types.NamespacedName{Namespace: policy.Namespace, Name: policy.Name}
				actualPolicyRefs[nsn.String()] = struct{}{}
			}
			assert.EqualValues(t, tc.expectedPolicyRefs, actualPolicyRefs)
		})
	}
}
