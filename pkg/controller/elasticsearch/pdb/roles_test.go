// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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
	ssetfixtures "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

func TestGetPrimaryRoleForPDB(t *testing.T) {
	tests := []struct {
		name     string
		roles    map[esv1.NodeRole]struct{}
		expected esv1.NodeRole
	}{
		{
			name:     "empty roles map",
			roles:    map[esv1.NodeRole]struct{}{},
			expected: "",
		},
		{
			name: "data role should be highest priority (most restrictive)",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataRole:   struct{}{},
				esv1.IngestRole: struct{}{},
				esv1.MLRole:     struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "master role should be second priority when no data roles",
			roles: map[esv1.NodeRole]struct{}{
				esv1.MasterRole: struct{}{},
				esv1.IngestRole: struct{}{},
				esv1.MLRole:     struct{}{},
			},
			expected: esv1.MasterRole,
		},
		{
			name: "data_hot role should match data role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataHotRole: struct{}{},
				esv1.IngestRole:  struct{}{},
				esv1.MLRole:      struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_warm role should match data role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataWarmRole: struct{}{},
				esv1.IngestRole:   struct{}{},
				esv1.MLRole:       struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_cold role should match data role	",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataColdRole: struct{}{},
				esv1.IngestRole:   struct{}{},
				esv1.MLRole:       struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_content role should match data role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataContentRole: struct{}{},
				esv1.IngestRole:      struct{}{},
				esv1.MLRole:          struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_frozen role should return data_frozen (has different disruption rules)",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataFrozenRole: struct{}{},
				esv1.IngestRole:     struct{}{},
				esv1.MLRole:         struct{}{},
			},
			expected: esv1.DataFrozenRole,
		},
		{
			name: "multiple data roles should match data role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.DataHotRole:  struct{}{},
				esv1.DataWarmRole: struct{}{},
				esv1.DataColdRole: struct{}{},
				esv1.IngestRole:   struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "master and data roles should return data role (data has higher priority)",
			roles: map[esv1.NodeRole]struct{}{
				esv1.MasterRole:    struct{}{},
				esv1.DataRole:      struct{}{},
				esv1.DataHotRole:   struct{}{},
				esv1.IngestRole:    struct{}{},
				esv1.MLRole:        struct{}{},
				esv1.TransformRole: struct{}{},
			},
			expected: esv1.DataRole,
		},
		{
			name: "only non-data roles should return first found",
			roles: map[esv1.NodeRole]struct{}{
				esv1.IngestRole:    struct{}{},
				esv1.MLRole:        struct{}{},
				esv1.TransformRole: struct{}{},
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ingest role should return ingest role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.IngestRole: struct{}{},
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ml role should return ml role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.MLRole: struct{}{},
			},
			expected: esv1.MLRole,
		},
		{
			name: "single transform role should return transform role",
			roles: map[esv1.NodeRole]struct{}{
				esv1.TransformRole: struct{}{},
			},
			expected: esv1.TransformRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPrimaryRoleForPDB(tt.roles)

			if !cmp.Equal(tt.expected, result) {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestReconcileRoleSpecificPDBs(t *testing.T) {
	rolePDB := func(esName, namespace string, role esv1.NodeRole, statefulSetNames []string, maxUnavailable int32) *policyv1.PodDisruptionBudget {
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PodDisruptionBudgetNameForRole(esName, role),
				Namespace: namespace,
				Labels:    map[string]string{label.ClusterNameLabelName: esName},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: maxUnavailable},
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
			// Sort for consistent test comparison
			sorted := make([]string, len(statefulSetNames))
			copy(sorted, statefulSetNames)
			slices.Sort(sorted)

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
						Values:   sorted,
					},
				},
			}
		}

		return pdb
	}

	defaultEs := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
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
					ssetfixtures.TestSset{
						Name:        "master1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Master:      true,
						Replicas:    1,
					}.Build(),
					ssetfixtures.TestSset{
						Name:        "data1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Data:        true,
						Replicas:    1,
					}.Build(),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data1"}, 0),
			},
		},
		{
			name: "existing default PDB: should delete it and create role-specific PDBs",
			args: args{
				initObjs: []client.Object{
					defaultPDB(),
				},
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Name: "master1", Namespace: "ns", ClusterName: "cluster", Master: true, Replicas: 1}.Build(),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// single node cluster should allow 1 pod to be unavailable
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 1),
			},
		},
		{
			name: "coordinating nodes: should be grouped together",
			args: args{
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{
						Name:        "coord1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Replicas:    1,
					}.Build(),
					ssetfixtures.TestSset{
						Name:        "coord2",
						Namespace:   "ns",
						ClusterName: "cluster",
						Replicas:    1,
					}.Build(),
					ssetfixtures.TestSset{
						Name:        "master1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Master:      true,
						Replicas:    1,
					}.Build(),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", "", []string{"coord1", "coord2"}, 0),
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
			},
		},
		{
			name: "mixed roles: should group StatefulSets sharing roles",
			args: args{
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{
						Name:        "master-data1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Master:      true,
						Data:        true,
						Replicas:    1,
					}.Build(),
					ssetfixtures.TestSset{
						Name:        "data-ingest1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Data:        true,
						Ingest:      true,
						Replicas:    1,
					}.Build(),
					ssetfixtures.TestSset{
						Name:        "ml1",
						Namespace:   "ns",
						ClusterName: "cluster",
						ML:          true,
						Replicas:    1,
					}.Build(),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.DataRole, []string{"master-data1", "data-ingest1"}, 0),
				rolePDB("cluster", "ns", esv1.MLRole, []string{"ml1"}, 0),
			},
		},
		{
			name: "PDB disabled in ES spec: should delete existing PDBs and not create new ones",
			args: func() args {
				es := esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec: esv1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{},
					},
				}
				return args{
					initObjs: []client.Object{
						withOwnerRef(defaultPDB(), es),
						withOwnerRef(rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0), es),
					},
					es: es,
					statefulSets: sset.StatefulSetList{
						ssetfixtures.TestSset{
							Name:        "master1",
							Namespace:   "ns",
							ClusterName: "cluster",
							Master:      true,
							Replicas:    1,
						}.Build(),
					},
				}
			}(),
			wantedPDBs: []*policyv1.PodDisruptionBudget{},
		},
		{
			name: "update existing role-specific PDBs",
			args: args{
				initObjs: []client.Object{
					// Existing PDB with different configuration
					&policyv1.PodDisruptionBudget{
						ObjectMeta: metav1.ObjectMeta{
							Name:      PodDisruptionBudgetNameForRole("cluster", esv1.MasterRole),
							Namespace: "ns",
							Labels:    map[string]string{label.ClusterNameLabelName: "cluster"},
						},
						Spec: policyv1.PodDisruptionBudgetSpec{
							MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 2}, // Wrong value
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									label.ClusterNameLabelName:     "cluster",
									label.StatefulSetNameLabelName: "old-master", // Wrong StatefulSet
								},
							},
						},
					},
				},
				es: defaultEs,
				statefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{
						Name:        "master1",
						Namespace:   "ns",
						ClusterName: "cluster",
						Master:      true,
						Replicas:    1,
					}.Build(),
				},
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 1),
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
				ssetfixtures.TestSset{
					Name:        "master1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Master:      true,
					Replicas:    1,
				}.Build(),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "ns",
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
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
			},
		},
		{
			name: "single coordinating node",
			statefulSets: []appsv1.StatefulSet{
				ssetfixtures.TestSset{
					Name:        "coord1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Replicas:    1,
				}.Build(),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coord",
						Namespace: "ns",
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
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
			},
		},
		{
			name: "separate roles - no shared roles",
			statefulSets: []appsv1.StatefulSet{
				ssetfixtures.TestSset{
					Name:        "master1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Master:      true,
					Replicas:    1,
				}.Build(),
				ssetfixtures.TestSset{
					Name:        "data1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Data:        true,
					Replicas:    1,
				}.Build(),
				ssetfixtures.TestSset{
					Name:        "ingest1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Ingest:      true,
					Replicas:    1,
				}.Build(),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "ns",
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
						Namespace: "ns",
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
						Namespace: "ns",
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
				ssetfixtures.TestSset{
					Name:        "master-data1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Master:      true,
					Data:        true,
					Replicas:    1,
				}.Build(),
				ssetfixtures.TestSset{
					Name:        "data-ingest1",
					Namespace:   "ns",
					ClusterName: "test-es",
					Data:        true,
					Ingest:      true,
					Replicas:    1,
				}.Build(),
				ssetfixtures.TestSset{
					Name:        "ml1",
					Namespace:   "ns",
					ClusterName: "test-es",
					ML:          true,
					Replicas:    1,
				}.Build(),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-data",
						Namespace: "ns",
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
						Namespace: "ns",
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
				ssetfixtures.TestSset{Name: "coord1", Namespace: "ns", ClusterName: "test-es", Replicas: 1}.Build(),
				ssetfixtures.TestSset{Name: "coord2", Namespace: "ns", ClusterName: "test-es", Replicas: 1}.Build(),
				ssetfixtures.TestSset{Name: "coord3", Namespace: "ns", ClusterName: "test-es", Replicas: 1}.Build(),
			},
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coord",
						Namespace: "ns",
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
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "ns",
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
				t.Fatalf("expectedRolePDBs: %v", err)
			}

			if !cmp.Equal(tt.expected, pdbs) {
				t.Errorf("expectedRolePDBs: PDBs do not match expected:\n%s", cmp.Diff(tt.expected, pdbs))
			}
		})
	}
}
