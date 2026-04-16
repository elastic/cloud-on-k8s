// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func Test_noUnsupportedSettings(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		expectErrors bool
	}{

		{
			name:         "no settings OK",
			es:           es("7.0.0"),
			expectErrors: false,
		},
		{
			name: "warn of unsupported setting FAIL",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.ClusterInitialMasterNodes: "foo",
								},
							},
							Count: 1,
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "warn of unsupported in multiple nodes FAIL",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.ClusterInitialMasterNodes: "foo",
								},
							},
						},
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.XPackSecurityTransportSslVerificationMode: "bar",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "non unsupported setting OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									"node.attr.box_type": "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "supported settings with unsupported string prefix OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.XPackSecurityTransportSslCertificateAuthorities: "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "settings are canonicalized before validation",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									"cluster": map[string]any{
										"initial_master_nodes": []string{"foo", "bar"},
									},
									"node.attr.box_type": "foo",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
		{
			name: "client auth setting optional OK",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.XPackSecurityHttpSslClientAuthentication: "optional",
								},
							},
						},
					},
				},
			},
			expectErrors: false,
		},
		{
			name: "client auth setting required yields forbidden field error for warning split",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.0.0",
					NodeSets: []esv1.NodeSet{
						{
							Config: &commonv1.Config{
								Data: map[string]any{
									esv1.XPackSecurityHttpSslClientAuthentication: "required",
								},
							},
						},
					},
				},
			},
			expectErrors: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := noUnsupportedSettings(tt.es)
			actualErrors := len(actual) > 0
			if tt.expectErrors != actualErrors {
				t.Errorf("failed noUnsupportedSettings(). Name: %v, actual %v, wanted: %v, value: %v", tt.name, actual, tt.expectErrors, tt.es.Spec.Version)
			}
		})
	}
}

func Test_settingsWarningsAndErrors(t *testing.T) {
	t.Run("forbidden reserved keys stay warnings only", func(t *testing.T) {
		es := esv1.Elasticsearch{
			Spec: esv1.ElasticsearchSpec{
				Version: "8.16.0",
				NodeSets: []esv1.NodeSet{{
					Count: 1,
					Config: &commonv1.Config{
						Data: map[string]any{
							esv1.ClusterInitialMasterNodes: "foo",
						},
					},
				}},
			},
		}
		warns, blocking := settingsWarningsAndErrors(es)
		require.Empty(t, blocking)
		require.Len(t, warns, 1)
		require.Contains(t, warns[0], unsupportedConfigErrMsg)
	})
	t.Run("mandatory client authentication is warning only", func(t *testing.T) {
		es := esv1.Elasticsearch{
			Spec: esv1.ElasticsearchSpec{
				Version: "8.16.0",
				NodeSets: []esv1.NodeSet{{
					Count: 1,
					Config: &commonv1.Config{
						Data: map[string]any{
							esv1.XPackSecurityHttpSslClientAuthentication: "required",
						},
					},
				}},
			},
		}
		warns, blocking := settingsWarningsAndErrors(es)
		require.Empty(t, blocking)
		require.Len(t, warns, 1)
		require.Contains(t, warns[0], unsupportedClientAuthenticationMsg)
	})
}

func Test_validZoneAwarenessAffinityWarnings(t *testing.T) {
	requiredAffinityWithExpression := func(key string, operator corev1.NodeSelectorOperator) *corev1.Affinity {
		return &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{Key: key, Operator: operator, Values: []string{"a"}},
							},
						},
					},
				},
			},
		}
	}

	topologyExprPath := func(nodeSetIndex int) string {
		return field.NewPath("spec").Child("nodeSets").Index(nodeSetIndex).Child("podTemplate", "spec", "affinity", "nodeAffinity", "requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms").Index(0).Child("matchExpressions").Index(0).String()
	}

	tests := []struct {
		name           string
		es             esv1.Elasticsearch
		expectedFields []string
	}{
		{
			name: "warns on not-in for zone-aware nodeset topology key",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression(esv1.DefaultZoneAwarenessTopologyKey, corev1.NodeSelectorOpNotIn),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(0)},
		},
		{
			name: "warns on not-in for non-zone-aware nodeset in zone-aware cluster",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "za",
							ZoneAwareness: &esv1.ZoneAwareness{
								TopologyKey: "topology.custom.io/rack",
							},
						},
						{
							Name: "plain",
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression("topology.custom.io/rack", corev1.NodeSelectorOpNotIn),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(1)},
		},
		{
			name: "does not warn when no nodeset enables zone awareness",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "plain",
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression(esv1.DefaultZoneAwarenessTopologyKey, corev1.NodeSelectorOpNotIn),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "does not warn on unrelated key",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression("nodepool", corev1.NodeSelectorOpNotIn),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "warns on does-not-exist on default zone-awareness topology key",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name:          "za",
							ZoneAwareness: &esv1.ZoneAwareness{},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression(esv1.DefaultZoneAwarenessTopologyKey, corev1.NodeSelectorOpDoesNotExist),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(0)},
		},
		{
			name: "warns on does-not-exist on non-zone-aware nodeset in zone-aware cluster",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "za",
							ZoneAwareness: &esv1.ZoneAwareness{
								TopologyKey: "topology.custom.io/rack",
							},
						},
						{
							Name: "plain",
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression("topology.custom.io/rack", corev1.NodeSelectorOpDoesNotExist),
								},
							},
						},
					},
				},
			},
			expectedFields: []string{topologyExprPath(1)},
		},
		{
			name: "does not warn on does-not-exist when no nodeset enables zone awareness",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "plain",
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Affinity: requiredAffinityWithExpression(esv1.DefaultZoneAwarenessTopologyKey, corev1.NodeSelectorOpDoesNotExist),
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := validZoneAwarenessAffinityWarnings(tt.es)
			actualFields := make([]string, len(warnings))
			for i, warning := range warnings {
				actualFields[i] = warning.Field
			}
			assert.ElementsMatch(t, tt.expectedFields, actualFields)
		})
	}
}

func Test_shorthandResourcesOverrideWarning(t *testing.T) {
	cpu := resource.MustParse("500m")
	memory := resource.MustParse("1Gi")

	tests := []struct {
		name     string
		es       esv1.Elasticsearch
		warnings int
	}{
		{
			name: "no warning when shorthand resources do not overlap pod template",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "default",
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU: &cpu,
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceMemory: memory,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			warnings: 0,
		},
		{
			name: "warning when shorthand resources overlap pod template",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{
							Name: "default",
							Resources: commonv1.Resources{
								Requests: commonv1.ResourceAllocations{
									CPU: &cpu,
								},
							},
							PodTemplate: corev1.PodTemplateSpec{
								Spec: corev1.PodSpec{
									Containers: []corev1.Container{
										{
											Name: esv1.ElasticsearchContainerName,
											Resources: corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU: cpu,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			warnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualWarnings := shorthandResourcesOverrideWarning(tt.es)
			require.Len(t, actualWarnings, tt.warnings)
			if tt.warnings > 0 {
				assert.Contains(t, actualWarnings[0].Detail, "overrides")
			}
		})
	}
}
