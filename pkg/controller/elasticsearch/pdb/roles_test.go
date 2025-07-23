package pdb

import (
	"context"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
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

func TestGetMostConservativeRole(t *testing.T) {
	tests := []struct {
		name     string
		roles    map[esv1.NodeRole]bool
		expected esv1.NodeRole
	}{
		{
			name:     "empty roles map",
			roles:    map[esv1.NodeRole]bool{},
			expected: "",
		},
		{
			name: "master role - most conservative",
			roles: map[esv1.NodeRole]bool{
				esv1.MasterRole: true,
				esv1.IngestRole: true,
				esv1.MLRole:     true,
			},
			expected: esv1.MasterRole,
		},
		{
			name: "data role - second most conservative",
			roles: map[esv1.NodeRole]bool{
				esv1.DataRole:   true,
				esv1.IngestRole: true,
				esv1.MLRole:     true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_hot role",
			roles: map[esv1.NodeRole]bool{
				esv1.DataHotRole: true,
				esv1.IngestRole:  true,
				esv1.MLRole:      true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_warm role",
			roles: map[esv1.NodeRole]bool{
				esv1.DataWarmRole: true,
				esv1.IngestRole:   true,
				esv1.MLRole:       true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_cold role",
			roles: map[esv1.NodeRole]bool{
				esv1.DataColdRole: true,
				esv1.IngestRole:   true,
				esv1.MLRole:       true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_content role",
			roles: map[esv1.NodeRole]bool{
				esv1.DataContentRole: true,
				esv1.IngestRole:      true,
				esv1.MLRole:          true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_frozen role",
			roles: map[esv1.NodeRole]bool{
				esv1.DataFrozenRole: true,
				esv1.IngestRole:     true,
				esv1.MLRole:         true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "multiple data roles - should return first found",
			roles: map[esv1.NodeRole]bool{
				esv1.DataHotRole:  true,
				esv1.DataWarmRole: true,
				esv1.DataColdRole: true,
				esv1.IngestRole:   true,
			},
			expected: esv1.DataRole,
		},
		{
			name: "master and data roles - master wins",
			roles: map[esv1.NodeRole]bool{
				esv1.MasterRole:    true,
				esv1.DataRole:      true,
				esv1.DataHotRole:   true,
				esv1.IngestRole:    true,
				esv1.MLRole:        true,
				esv1.TransformRole: true,
			},
			expected: esv1.MasterRole,
		},
		{
			name: "only non-data roles - returns first found",
			roles: map[esv1.NodeRole]bool{
				esv1.IngestRole:    true,
				esv1.MLRole:        true,
				esv1.TransformRole: true,
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ingest role",
			roles: map[esv1.NodeRole]bool{
				esv1.IngestRole: true,
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ml role",
			roles: map[esv1.NodeRole]bool{
				esv1.MLRole: true,
			},
			expected: esv1.MLRole,
		},
		{
			name: "single transform role",
			roles: map[esv1.NodeRole]bool{
				esv1.TransformRole: true,
			},
			expected: esv1.TransformRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMostConservativeRole(tt.roles)

			if !cmp.Equal(tt.expected, result) {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestReconcileRoleSpecificPDBs(t *testing.T) {
	// Helper function to create a default PDB (single cluster-wide PDB)
	defaultPDB := func(esName, namespace string) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      esv1.DefaultPodDisruptionBudget(esName),
				Namespace: namespace,
				Labels:    map[string]string{label.ClusterNameLabelName: esName},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						label.ClusterNameLabelName: esName,
					},
				},
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			},
		}
	}

	// Helper function to create a role-specific PDB
	rolePDB := func(esName, namespace string, role esv1.NodeRole, statefulSetNames []string) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RolePodDisruptionBudgetName(esName, role),
				Namespace: namespace,
				Labels:    map[string]string{label.ClusterNameLabelName: esName},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0}, // Default for unknown health
			},
		}

		// Set selector based on number of StatefulSets
		if len(statefulSetNames) == 1 {
			// Single StatefulSet - use MatchLabels
			pdb.Spec.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					label.ClusterNameLabelName:     esName,
					label.StatefulSetNameLabelName: statefulSetNames[0],
				},
			}
		} else {
			// Multiple StatefulSets - use MatchExpressions
			pdb.Spec.Selector = &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      label.ClusterNameLabelName,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{esName},
					},
					{
						Key:      label.StatefulSetNameLabelName,
						Operator: metav1.LabelSelectorOpIn,
						Values: func() []string {
							// Sort for consistent test comparison
							sorted := make([]string, len(statefulSetNames))
							copy(sorted, statefulSetNames)
							slices.Sort(sorted)
							return sorted
						}(),
					},
				},
			}
		}

		return pdb
	}

	defaultEs := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}

	type args struct {
		initObjs     []client.Object
		es           esv1.Elasticsearch
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name       string
		args       args
		wantedPDBs []*policyv1.PodDisruptionBudget
	}{
		{
			name: "no existing PDBs: should create role-specific PDBs",
			args: args{
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
					createStatefulSetWithRoles("data1", []esv1.NodeRole{esv1.DataRole}),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master1"}),
				rolePDB("test-cluster", "default", esv1.DataRole, []string{"data1"}),
			},
		},
		{
			name: "existing default PDB: should delete it and create role-specific PDBs",
			args: args{
				initObjs: []client.Object{
					defaultPDB("test-cluster", "default"),
				},
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master1"}),
			},
		},
		{
			name: "coordinating nodes: should be grouped together",
			args: args{
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					createStatefulSetWithRoles("coord1", []esv1.NodeRole{}),
					createStatefulSetWithRoles("coord2", []esv1.NodeRole{}),
					createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Coordinating nodes grouped together with empty role (gets "coord" suffix)
				rolePDB("test-cluster", "default", "", []string{"coord1", "coord2"}),
				rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master1"}),
			},
		},
		{
			name: "mixed roles: should group StatefulSets sharing roles",
			args: args{
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					createStatefulSetWithRoles("master-data1", []esv1.NodeRole{esv1.MasterRole, esv1.DataRole}),
					createStatefulSetWithRoles("data-ingest1", []esv1.NodeRole{esv1.DataRole, esv1.IngestRole}),
					createStatefulSetWithRoles("ml1", []esv1.NodeRole{esv1.MLRole}),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// master-data1 and data-ingest1 should be grouped because they share DataRole
				// Most conservative role is MasterRole, so PDB uses master role
				rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master-data1", "data-ingest1"}),
				// ml1 gets its own PDB
				rolePDB("test-cluster", "default", esv1.MLRole, []string{"ml1"}),
			},
		},
		{
			name: "PDB disabled in ES spec: should delete existing PDBs and not create new ones",
			args: func() args {
				es := esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
					Spec: esv1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{},
					},
				}
				return args{
					initObjs: []client.Object{
						withOwnerRef(defaultPDB("test-cluster", "default"), es),
						withOwnerRef(rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master1"}), es),
					},
					es: es,
					statefulSets: sset.StatefulSetList{
						createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
					},
				}
			}(),
			wantedPDBs: []*policyv1.PodDisruptionBudget{}, // No PDBs should be created
		},
		{
			name: "update existing role-specific PDBs",
			args: args{
				initObjs: []client.Object{
					// Existing PDB with different configuration
					&policyv1.PodDisruptionBudget{
						ObjectMeta: metav1.ObjectMeta{
							Name:      RolePodDisruptionBudgetName("test-cluster", esv1.MasterRole),
							Namespace: "default",
							Labels:    map[string]string{label.ClusterNameLabelName: "test-cluster"},
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2}, // Wrong value
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									label.ClusterNameLabelName:     "test-cluster",
									label.StatefulSetNameLabelName: "old-master", // Wrong StatefulSet
								},
							},
						},
					},
				},
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					createStatefulSetWithRoles("master1", []esv1.NodeRole{esv1.MasterRole}),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Should be updated with correct configuration
				rolePDB("test-cluster", "default", esv1.MasterRole, []string{"master1"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{
				Group:   "policy",
				Version: "v1",
			}})
			restMapper.Add(
				schema.GroupVersionKind{
					Group:   "policy",
					Version: "v1",
					Kind:    "PodDisruptionBudget",
				}, meta.RESTScopeNamespace)
			c := fake.NewClientBuilder().
				WithScheme(clientgoscheme.Scheme).
				WithRESTMapper(restMapper).
				WithObjects(tt.args.initObjs...).
				Build()

			// Create metadata
			meta := metadata.Propagate(&tt.args.es, metadata.Metadata{Labels: tt.args.es.GetIdentityLabels()})

			err := reconcileRoleSpecificPDBs(context.Background(), c, tt.args.es, tt.args.statefulSets, meta)
			require.NoError(t, err)

			var retrievedPDBs policyv1.PodDisruptionBudgetList
			err = c.List(context.Background(), &retrievedPDBs, client.InNamespace(tt.args.es.Namespace))
			require.NoError(t, err)

			require.Equal(t, len(tt.wantedPDBs), len(retrievedPDBs.Items), "Expected %d PDBs, got %d", len(tt.wantedPDBs), len(retrievedPDBs.Items))

			for _, expectedPDB := range tt.wantedPDBs {
				// Find the matching PDB in the retrieved list
				idx := slices.IndexFunc(retrievedPDBs.Items, func(pdb policyv1.PodDisruptionBudget) bool {
					return pdb.Name == expectedPDB.Name
				})
				require.NotEqual(t, -1, idx, "Expected PDB %s should exist", expectedPDB.Name)
				actualPDB := &retrievedPDBs.Items[idx]

				// Verify key fields match (ignore metadata like resourceVersion, etc.)
				require.Equal(t, expectedPDB.Spec.MaxUnavailable, actualPDB.Spec.MaxUnavailable, "MaxUnavailable should match for PDB %s", expectedPDB.Name)
				require.Equal(t, expectedPDB.Spec.Selector, actualPDB.Spec.Selector, "Selector should match for PDB %s", expectedPDB.Name)
				require.Equal(t, expectedPDB.Labels[label.ClusterNameLabelName], actualPDB.Labels[label.ClusterNameLabelName], "Cluster label should match for PDB %s", expectedPDB.Name)
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
