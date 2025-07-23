package pdb

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

// Helper function to create a StatefulSet with specific roles in pod template labels
func createStatefulSetWithRoles(name string, roles []esv1.NodeRole) appsv1.StatefulSet {
	labels := make(map[string]string)

	// Add role labels based on the roles provided
	for _, role := range roles {
		switch role {
		case esv1.MasterRole:
			labels[string(label.NodeTypesMasterLabelName)] = "true"
		case esv1.DataRole:
			labels[string(label.NodeTypesDataLabelName)] = "true"
		case esv1.IngestRole:
			labels[string(label.NodeTypesIngestLabelName)] = "true"
		case esv1.MLRole:
			labels[string(label.NodeTypesMLLabelName)] = "true"
		case esv1.TransformRole:
			labels[string(label.NodeTypesTransformLabelName)] = "true"
		case esv1.RemoteClusterClientRole:
			labels[string(label.NodeTypesRemoteClusterClientLabelName)] = "true"
		case esv1.DataHotRole:
			labels[string(label.NodeTypesDataHotLabelName)] = "true"
		case esv1.DataWarmRole:
			labels[string(label.NodeTypesDataWarmLabelName)] = "true"
		case esv1.DataColdRole:
			labels[string(label.NodeTypesDataColdLabelName)] = "true"
		case esv1.DataContentRole:
			labels[string(label.NodeTypesDataContentLabelName)] = "true"
		case esv1.DataFrozenRole:
			labels[string(label.NodeTypesDataFrozenLabelName)] = "true"
		}
	}

	return appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
			},
		},
	}
}

func TestMergeGroupsWithRole(t *testing.T) {
	tests := []struct {
		name     string
		groups   [][]appsv1.StatefulSet
		role     esv1.NodeRole
		expected [][]appsv1.StatefulSet
	}{
		{
			name: "no groups have the role",
			groups: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole})},
			},
			role: esv1.DataRole,
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole})},
			},
		},
		{
			name: "only one group has the role",
			groups: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole})},
			},
			role: esv1.DataRole,
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole})},
			},
		},
		{
			name: "two groups have the role - should merge",
			groups: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole})},
				{createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.MLRole})},
			},
			role: esv1.DataRole,
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}), createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole})},
				{createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.MLRole})},
			},
		},
		{
			name: "three groups have the role - should merge all",
			groups: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole})},
				{createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole})},
				{createStatefulSetWithRoles("sset4", []esv1.NodeRole{esv1.DataRole})},
			},
			role: esv1.DataRole,
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole}), createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}), createStatefulSetWithRoles("sset4", []esv1.NodeRole{esv1.DataRole})},
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
			},
		},
		{
			name:     "empty groups",
			groups:   [][]appsv1.StatefulSet{},
			role:     esv1.DataRole,
			expected: [][]appsv1.StatefulSet{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeGroupsWithRole(tt.groups, tt.role)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d groups, got %d", len(tt.expected), len(result))
				return
			}

			if !cmp.Equal(tt.expected, result) {
				t.Errorf("Expected %v\ngot %v", tt.expected, result)
			}
		})
	}
}

func TestGroupStatefulSetsByConnectedRoles(t *testing.T) {
	tests := []struct {
		name         string
		statefulSets []appsv1.StatefulSet
		expected     [][]appsv1.StatefulSet
	}{
		{
			name:         "empty input",
			statefulSets: []appsv1.StatefulSet{},
			expected:     nil,
		},
		{
			name: "single StatefulSet",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole}),
			},
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
			},
		},
		{
			name: "two StatefulSets with no shared roles",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole}),
				createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole}),
			},
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole})},
				{createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.IngestRole})},
			},
		},
		{
			name: "two StatefulSets with shared role",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}),
				createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}),
			},
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}), createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole})},
			},
		},
		{
			name: "complex scenario - transitive connections",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}),
				createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}),
				createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.IngestRole}),
				createStatefulSetWithRoles("sset4", []esv1.NodeRole{esv1.MLRole}),
			},
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("sset1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}), createStatefulSetWithRoles("sset2", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}), createStatefulSetWithRoles("sset3", []esv1.NodeRole{esv1.IngestRole})},
				{createStatefulSetWithRoles("sset4", []esv1.NodeRole{esv1.MLRole})},
			},
		},
		{
			name: "coordinating nodes (no roles)",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("coord1", []esv1.NodeRole{}),
				createStatefulSetWithRoles("coord2", []esv1.NodeRole{}),
				createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
			},
			expected: [][]appsv1.StatefulSet{
				// Coordinating nodes should be grouped together to avoid PDB naming conflicts
				{
					createStatefulSetWithRoles("coord1", []esv1.NodeRole{}),
					createStatefulSetWithRoles("coord2", []esv1.NodeRole{}),
				},
				{createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole})},
			},
		},
		{
			name: "multiple data tier roles",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("hot1", []esv1.NodeRole{esv1.DataHotRole}),
				createStatefulSetWithRoles("warm1", []esv1.NodeRole{esv1.DataWarmRole}),
				createStatefulSetWithRoles("cold1", []esv1.NodeRole{esv1.DataColdRole}),
				createStatefulSetWithRoles("mixed1", []esv1.NodeRole{esv1.DataHotRole, esv1.DataWarmRole}),
			},
			expected: [][]appsv1.StatefulSet{
				{createStatefulSetWithRoles("hot1", []esv1.NodeRole{esv1.DataHotRole}), createStatefulSetWithRoles("mixed1", []esv1.NodeRole{esv1.DataHotRole, esv1.DataWarmRole}), createStatefulSetWithRoles("warm1", []esv1.NodeRole{esv1.DataWarmRole})},
				{createStatefulSetWithRoles("cold1", []esv1.NodeRole{esv1.DataColdRole})},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to StatefulSetList
			statefulSetList := sset.StatefulSetList{}
			for _, s := range tt.statefulSets {
				statefulSetList = append(statefulSetList, s)
			}

			result := groupStatefulSetsByConnectedRoles(statefulSetList)

			if !cmp.Equal(result, tt.expected) {
				t.Errorf("Result does not match expected:\n%s", cmp.Diff(tt.expected, result))
			}
		})
	}
}

func TestExpectedRolePDBs(t *testing.T) {
	tests := []struct {
		name         string
		statefulSets []appsv1.StatefulSet
		expected     []*policyv1.PodDisruptionBudget
	}{
		{
			name:         "empty input",
			statefulSets: []appsv1.StatefulSet{},
			expected:     []*policyv1.PodDisruptionBudget{},
		},
		{
			name: "single master nodeset",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "master1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "single coordinating node (no roles)",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("coord1", []esv1.NodeRole{}),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coord",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "coord1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "separate roles - no shared roles",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
				createStatefulSetWithRoles("data1", []esv1.NodeRole{esv1.DataRole}),
				createStatefulSetWithRoles("ingest1", []esv1.NodeRole{esv1.IngestRole}),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "master1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-data",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "data1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-ingest",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "ingest1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "shared roles - should be grouped",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("master-data1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}),
				createStatefulSetWithRoles("data-ingest1", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}),
				createStatefulSetWithRoles("ml1", []esv1.NodeRole{esv1.MLRole}),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"data-ingest1", "master-data1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-ml",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								label.ClusterNameLabelName:     "test-es",
								label.StatefulSetNameLabelName: "ml1",
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "multiple coordinating nodeSets",
			statefulSets: []appsv1.StatefulSet{
				createStatefulSetWithRoles("coord1", []esv1.NodeRole{}),
				createStatefulSetWithRoles("coord2", []esv1.NodeRole{}),
				createStatefulSetWithRoles("coord3", []esv1.NodeRole{}),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coord",
						Namespace: "default",
						Labels: map[string]string{
							label.ClusterNameLabelName: "test-es",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1",
								Kind:               "Elasticsearch",
								Name:               "test-es",
								Controller:         ptr.To[bool](true),
								BlockOwnerDeletion: ptr.To[bool](true),
							},
						},
					},
					Spec: policyv1.PodDisruptionBudgetSpec{
						Selector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"coord1", "coord2", "coord3"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test Elasticsearch resource
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "default",
				},
				Spec: esv1.ElasticsearchSpec{
					Version: "8.0.0",
				},
			}

			statefulSetList := sset.StatefulSetList{}
			for _, s := range tt.statefulSets {
				statefulSetList = append(statefulSetList, s)
			}

			meta := metadata.Metadata{
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/cluster-name": "test-es",
				},
			}

			pdbs, err := expectedRolePDBs(es, statefulSetList, meta)
			if err != nil {
				t.Fatalf("expectedRolePDBs returned error: %v", err)
			}

			if !cmp.Equal(tt.expected, pdbs) {
				t.Errorf("Result does not match expected:\n%s", cmp.Diff(tt.expected, pdbs))
			}

			// // Run custom validation if provided
			// if tt.validation != nil {
			// 	tt.validation(t, pdbs)
			// }

			// // Basic validation for all PDBs
			// for i, pdb := range pdbs {
			// 	if pdb == nil {
			// 		t.Errorf("PDB %d is nil", i)
			// 		continue
			// 	}
			// 	// Verify PDB has proper metadata
			// 	if pdb.Namespace != "default" {
			// 		t.Errorf("Expected PDB namespace 'default', got '%s'", pdb.Namespace)
			// 	}
			// 	if pdb.Labels == nil || pdb.Labels["elasticsearch.k8s.elastic.co/cluster-name"] != "test-es" {
			// 		t.Errorf("PDB missing proper cluster label")
			// 	}
			// 	// Verify PDB has selector
			// 	if pdb.Spec.Selector == nil {
			// 		t.Errorf("PDB %s missing selector", pdb.Name)
			// 	}
			// 	// Verify MaxUnavailable is set
			// 	if pdb.Spec.MaxUnavailable == nil {
			// 		t.Errorf("PDB %s missing MaxUnavailable", pdb.Name)
			// 	}
			// }
		})
	}
}
