// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"slices"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	_ "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	ssetfixtures "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	_ "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

func TestGetPrimaryRoleForPDB(t *testing.T) {
	tests := []struct {
		name     string
		roles    func() sets.Set[esv1.NodeRole]
		expected esv1.NodeRole
	}{
		{
			name:     "empty roles map",
			roles:    func() sets.Set[esv1.NodeRole] { return sets.New[esv1.NodeRole]() },
			expected: "",
		},
		{
			name: "data role should be highest priority (most restrictive)",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "master role should be second priority when no data roles",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.MasterRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.MasterRole,
		},
		{
			name: "data_hot role should match data role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataHotRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_warm role should match data role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataWarmRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_cold role should match data role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataColdRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_content role should match data role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataContentRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "data_frozen role should return data_frozen (has different disruption rules)",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataFrozenRole, esv1.IngestRole, esv1.MLRole)
			},
			expected: esv1.DataFrozenRole,
		},
		{
			name: "multiple data roles should match data role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.DataHotRole, esv1.DataWarmRole, esv1.DataColdRole, esv1.IngestRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "master and data roles should return data role (data has higher priority)",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.MasterRole, esv1.DataRole, esv1.DataHotRole, esv1.IngestRole, esv1.MLRole, esv1.TransformRole)
			},
			expected: esv1.DataRole,
		},
		{
			name: "only non-data roles should return first found",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.IngestRole, esv1.MLRole, esv1.TransformRole)
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ingest role should return ingest role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.IngestRole)
			},
			expected: esv1.IngestRole,
		},
		{
			name: "single ml role should return ml role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.MLRole)
			},
			expected: esv1.MLRole,
		},
		{
			name: "single transform role should return transform role",
			roles: func() sets.Set[esv1.NodeRole] {
				return sets.New(esv1.TransformRole)
			},
			expected: esv1.TransformRole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPrimaryRoleForPDB(tt.roles())

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
				Name:      esv1.PodDisruptionBudgetNameForRole(esName, string(role)),
				Namespace: namespace,
				Labels:    map[string]string{label.ClusterNameLabelName: esName},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: maxUnavailable},
			},
		}

		// Sort for consistent test comparison
		sorted := make([]string, len(statefulSetNames))
		copy(sorted, statefulSetNames)
		slices.Sort(sorted)

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

		return pdb
	}

	defaultEs := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
		Spec: esv1.ElasticsearchSpec{
			Version: "9.0.1",
		},
	}

	defaultHealthyES := defaultEs.DeepCopy()
	defaultHealthyES.Status.Health = esv1.ElasticsearchGreenHealth

	type args struct {
		initObjs []client.Object
		es       esv1.Elasticsearch
		builder  Builder
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
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master1", 1, esv1.MasterRole).
					WithNodeSet("data1", 1, esv1.DataRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Unhealthy es cluster; 0 disruptions allowed
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data1"}, 0),
			},
		},
		{
			name: "no existing PDBs: should create role-specific PDBs with data roles grouped",
			args: args{
				es: *defaultHealthyES,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master-data1", 2, esv1.MasterRole, esv1.DataRole).
					WithNodeSet("data2", 2, esv1.DataHotRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data2", "master-data1"}, 1),
			},
		},
		{
			name: "no existing PDBs: should create role-specific PDBs with data roles grouped, but no disruptions allowed because single master node",
			args: args{
				es: *defaultHealthyES,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master-data1", 1, esv1.MasterRole, esv1.DataRole).
					WithNodeSet("data2", 2, esv1.DataHotRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data2", "master-data1"}, 0),
			},
		},
		{
			name: "existing default PDB: should delete it and create role-specific PDBs",
			args: args{
				initObjs: []client.Object{
					defaultPDB(),
				},
				es: *defaultHealthyES,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master1", 1, esv1.MasterRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// single node cluster should allow 1 pod to be unavailable when cluster is healthy.
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 1),
			},
		},
		{
			name: "create pdb with coordinating nodes: no existing PDBs",
			args: args{
				es: defaultEs,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("coord1", 1, "").
					WithNodeSet("coord2", 1, "").
					WithNodeSet("master1", 1, esv1.MasterRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Unhealthy es cluster; 0 disruptions allowed
				rolePDB("cluster", "ns", "", []string{"coord1", "coord2"}, 0),
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
			},
		},
		{
			name: "mixed roles: should group StatefulSets sharing roles",
			args: args{
				es: defaultEs,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master-data1", 1, esv1.MasterRole, esv1.DataRole).
					WithNodeSet("data-ingest1", 1, esv1.DataRole, esv1.IngestRole).
					WithNodeSet("ml1", 1, esv1.MLRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Unhealthy es cluster; 0 disruptions allowed
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
					builder: NewBuilder("cluster").
						WithNamespace("ns").
						WithNodeSet("master1", 1, esv1.MasterRole),
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
							Name:      esv1.PodDisruptionBudgetNameForRole("cluster", string(esv1.MasterRole)),
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
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master1", 1, esv1.MasterRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Unhealthy es cluster; 0 disruptions allowed
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
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

			resourcesList, err := tt.args.builder.BuildResourcesList()
			require.NoError(t, err)

			statefulSets := tt.args.builder.GetStatefulSets()

			err = reconcileRoleSpecificPDBs(context.Background(), c, tt.args.es, statefulSets, resourcesList, meta)
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
				require.NotEqual(t, -1, idx, "Expected PDB %s should exist, found: %+v", expectedPDB.Name, retrievedPDBs.Items)
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
	defaultUnhealthyES := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es",
			Namespace: "ns",
		},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.0.0",
		},
		Status: esv1.ElasticsearchStatus{
			Health: esv1.ElasticsearchUnknownHealth,
		},
	}

	defaultHealthyES := defaultUnhealthyES.DeepCopy()
	defaultHealthyES.Status.Health = esv1.ElasticsearchGreenHealth

	tests := []struct {
		name     string
		es       esv1.Elasticsearch
		builder  Builder
		expected []*policyv1.PodDisruptionBudget
	}{
		{
			name:     "empty input",
			es:       *defaultHealthyES,
			builder:  NewBuilder("test-es").WithNamespace("ns").WithVersion("8.0.0"),
			expected: []*policyv1.PodDisruptionBudget{},
		},
		{
			name: "single node cluster; role doesn't matter; 1 disruption",
			es:   *defaultHealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole),
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
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"master1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
			},
		},
		{
			name: "multiple coordinating nodes; healthy es; 1 disruption allowed",
			es:   *defaultHealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("coord1", 2, esv1.CoordinatingRole),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coordinating",
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
									Values:   []string{"coord1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
					},
				},
			},
		},
		{
			name: "separate roles - no shared roles",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole).
				WithNodeSet("data1", 1, esv1.DataRole).
				WithNodeSet("ingest1", 1, esv1.IngestRole),
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
									Values:   []string{"data1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
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
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"master1"},
								},
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
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"ingest1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "existing PDB with different selector: should be updated",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole),
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
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"master1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "multiple coordinating nodeSets",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("coord1", 1, esv1.CoordinatingRole).
				WithNodeSet("coord2", 1, esv1.CoordinatingRole).
				WithNodeSet("coord3", 1, esv1.CoordinatingRole),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coordinating",
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
		{
			name: "shared roles - should be grouped",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master-data1", 1, esv1.MasterRole, esv1.DataRole).
				WithNodeSet("data-ingest1", 1, esv1.DataRole, esv1.IngestRole).
				WithNodeSet("ml1", 1, esv1.MLRole),
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
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      label.ClusterNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"test-es"},
								},
								{
									Key:      label.StatefulSetNameLabelName,
									Operator: metav1.LabelSelectorOpIn,
									Values:   []string{"ml1"},
								},
							},
						},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
		},
		{
			name: "multiple coordinating nodeSets",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("coord1", 1, esv1.CoordinatingRole).
				WithNodeSet("coord2", 1, esv1.CoordinatingRole).
				WithNodeSet("coord3", 1, esv1.CoordinatingRole),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coordinating",
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
		{
			name: "multiple coordinating nodeSets",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("coord1", 1, esv1.CoordinatingRole).
				WithNodeSet("coord2", 1, esv1.CoordinatingRole).
				WithNodeSet("coord3", 1, esv1.CoordinatingRole),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-coordinating",
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
			meta := metadata.Metadata{
				Labels: map[string]string{
					"elasticsearch.k8s.elastic.co/cluster-name": "test-es",
				},
			}

			resourcesList, err := tt.builder.BuildResourcesList()
			require.NoError(t, err)

			statefulSetList := tt.builder.GetStatefulSets()

			pdbs, err := expectedRolePDBs(tt.es, statefulSetList, resourcesList, meta)
			if err != nil {
				t.Fatalf("expectedRolePDBs: %v", err)
			}

			if !cmp.Equal(tt.expected, pdbs) {
				t.Errorf("expectedRolePDBs: PDBs do not match expected:\n%s", cmp.Diff(tt.expected, pdbs))
			}
		})
	}
}

func Test_allowedDisruptionsForRole(t *testing.T) {
	type args struct {
		es                esv1.Elasticsearch
		role              []esv1.NodeRole
		statefulSetsInPDB sset.StatefulSetList
		allStatefulSets   sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want int32
	}{
		{
			name: "no health reported: 0 disruptions allowed for any role",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{}},
				role:            []esv1.NodeRole{esv1.MasterRole, esv1.IngestRole, esv1.TransformRole, esv1.MLRole, esv1.DataFrozenRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "Unknown health reported: 0 disruptions allowed for any role",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchUnknownHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole, esv1.IngestRole, esv1.TransformRole, esv1.MLRole, esv1.DataFrozenRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "yellow health: 0 disruptions allowed for data nodes",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchYellowHealth}},
				role:            []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "yellow health: 1 disruption allowed for master/ingest/transform/ml/data_frozen",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchYellowHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole, esv1.IngestRole, esv1.TransformRole, esv1.MLRole, esv1.DataFrozenRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3}.Build()},
			},
			want: 1,
		},
		{
			name: "red health: 0 disruptions allowed for any role",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchRedHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole, esv1.IngestRole, esv1.TransformRole, esv1.MLRole, esv1.DataFrozenRole, esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "green health: 1 disruption allowed for any role",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole, esv1.IngestRole, esv1.TransformRole, esv1.MLRole, esv1.DataFrozenRole, esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "single-node cluster (not high-available): 1 disruption allowed",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 1, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "green health but only 1 master: 0 disruptions allowed for master role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.MasterRole},
				statefulSetsInPDB: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
				},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
					ssetfixtures.TestSset{Replicas: 2, Ingest: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 master: 1 disruption allowed for data role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
				},
			},
			want: 1,
		},
		{
			name: "green health but only 1 data node: 0 disruptions allowed for data role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 ingest node: 0 disruptions allowed for ingest role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.IngestRole},
				statefulSetsInPDB: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: true, Ingest: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Ingest: true, Data: true}.Build(),
				},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: true, Ingest: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Ingest: true, Data: true}.Build(),
					ssetfixtures.TestSset{Replicas: 1, DataFrozen: true}.Build(),
				},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, role := range tt.args.role {
				if got := allowedDisruptionsForRole(tt.args.es, role, tt.args.statefulSetsInPDB, tt.args.allStatefulSets); got != tt.want {
					t.Errorf("allowedDisruptionsForRole() = %v, want %v for role: %s", got, tt.want, role)
				}
			}
		})
	}
}

func TestGroupBySharedRoles(t *testing.T) {
	tests := []struct {
		name           string
		builder        Builder
		want           map[esv1.NodeRole][]appsv1.StatefulSet
		wantSTSToRoles map[string]set.StringSet
	}{
		{
			name:           "empty statefulsets",
			builder:        NewBuilder("test-es"),
			want:           map[esv1.NodeRole][]appsv1.StatefulSet{},
			wantSTSToRoles: nil,
		},
		{
			name: "single statefulset with no roles",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("coordinating", 1, esv1.CoordinatingRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.CoordinatingRole: {
					ssetfixtures.TestSset{Name: "coordinating", ClusterName: "test-es", Version: "9.0.1"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"coordinating": set.Make(""),
			},
		},
		{
			name: "all statefulsets with different roles",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("master", 1, esv1.MasterRole).
				WithNodeSet("ingest", 1, esv1.IngestRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.MasterRole: {
					ssetfixtures.TestSset{Name: "master", Master: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.IngestRole: {
					ssetfixtures.TestSset{Name: "ingest", Ingest: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"master": set.Make("master"),
				"ingest": set.Make("ingest"),
			},
		},
		{
			name: "statefulsets with shared roles are grouped properly",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("master", 1, esv1.MasterRole, esv1.DataRole).
				WithNodeSet("data", 1, esv1.DataRole).
				WithNodeSet("ingest", 1, esv1.IngestRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "master", Master: true, Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data", Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.IngestRole: {
					ssetfixtures.TestSset{Name: "ingest", Ingest: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"master": set.Make("master", "data"),
				"data":   set.Make("data"),
				"ingest": set.Make("ingest"),
			},
		},
		{
			name: "statefulsets with multiple shared roles in multiple groups, and data* roles are grouped properly",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("master", 1, esv1.MasterRole, esv1.DataRole).
				WithNodeSet("data", 1, esv1.DataRole).
				WithNodeSet("data_hot", 1, esv1.DataHotRole).
				WithNodeSet("data_warm", 1, esv1.DataWarmRole).
				WithNodeSet("data_cold", 1, esv1.DataColdRole).
				WithNodeSet("data_frozen", 1, esv1.DataFrozenRole).
				WithNodeSet("ingest", 1, esv1.IngestRole, esv1.MLRole).
				WithNodeSet("ml", 1, esv1.MLRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "master", Master: true, Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data", Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_hot", DataHot: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_warm", DataWarm: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_cold", DataCold: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.DataFrozenRole: {
					ssetfixtures.TestSset{Name: "data_frozen", DataFrozen: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.IngestRole: {
					ssetfixtures.TestSset{Name: "ingest", Ingest: true, ML: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "ml", ML: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"master":      set.Make("master", "data"),
				"data":        set.Make("data"),
				"data_hot":    set.Make("data"),
				"data_warm":   set.Make("data"),
				"data_cold":   set.Make("data"),
				"data_frozen": set.Make("data_frozen"),
				"ingest":      set.Make("ingest", "ml"),
				"ml":          set.Make("ml"),
			},
		},
		{
			name: "coordinating nodes (no roles) in separate group",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("data", 1, esv1.DataRole).
				WithNodeSet("coordinating1", 1, esv1.CoordinatingRole).
				WithNodeSet("coordinating2", 1, esv1.CoordinatingRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "data", Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.CoordinatingRole: {
					ssetfixtures.TestSset{Name: "coordinating1", Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "coordinating2", Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"data":          set.Make("data"),
				"coordinating1": set.Make(""),
				"coordinating2": set.Make(""),
			},
		},
		{
			name: "statefulsets with multiple roles respect priority order",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("master-data-ingest", 1, esv1.MasterRole, esv1.DataRole, esv1.IngestRole).
				WithNodeSet("data-ingest", 1, esv1.DataRole, esv1.IngestRole).
				WithNodeSet("ingest-only", 1, esv1.IngestRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "master-data-ingest", Master: true, Data: true, Ingest: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data-ingest", Data: true, Ingest: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "ingest-only", Ingest: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"master-data-ingest": set.Make("master", "data", "ingest"),
				"data-ingest":        set.Make("data", "ingest"),
				"ingest-only":        set.Make("ingest"),
			},
		},
		{
			name: "mixed data role types are properly collapsed even with generic data role existing",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("data", 1, esv1.DataRole).
				WithNodeSet("data_hot", 1, esv1.DataHotRole).
				WithNodeSet("data_content", 1, esv1.DataContentRole).
				WithNodeSet("master", 1, esv1.MasterRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.MasterRole: {
					ssetfixtures.TestSset{Name: "master", Master: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "data", Data: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_hot", DataHot: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_content", DataContent: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"data":         set.Make("data"),
				"data_hot":     set.Make("data"),
				"data_content": set.Make("data"),
				"master":       set.Make("master"),
			},
		},
		{
			name: "data roles without generic data role do not maintain separate groups",
			builder: NewBuilder("test-es").
				WithVersion("9.0.1").
				WithNodeSet("data_hot", 1, esv1.DataHotRole).
				WithNodeSet("data_cold", 1, esv1.DataColdRole).
				WithNodeSet("master", 1, esv1.MasterRole),
			want: map[esv1.NodeRole][]appsv1.StatefulSet{
				esv1.MasterRole: {
					ssetfixtures.TestSset{Name: "master", Master: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
				esv1.DataRole: {
					ssetfixtures.TestSset{Name: "data_hot", DataHot: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
					ssetfixtures.TestSset{Name: "data_cold", DataCold: true, Version: "9.0.1", ClusterName: "test-es"}.Build(),
				},
			},
			wantSTSToRoles: map[string]set.StringSet{
				"data_hot":  set.Make("data"),
				"data_cold": set.Make("data"),
				"master":    set.Make("master"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resourcesList nodespec.ResourcesList
			var err error
			resourcesList, err = tt.builder.BuildResourcesList()
			require.NoError(t, err)

			v := version.MustParse(tt.builder.Elasticsearch.Spec.Version)
			stss := tt.builder.GetStatefulSets()

			got, gotSTSToRoles, err := groupBySharedRoles(stss, resourcesList, v)
			assert.NoError(t, err)

			if !cmp.Equal(gotSTSToRoles, tt.wantSTSToRoles) {
				t.Errorf("gotSTSToRoles: diff = %s", cmp.Diff(gotSTSToRoles, tt.wantSTSToRoles))
			}

			// Check that the number of groups matches
			assert.Equal(t, len(tt.want), len(got), "Expected %d groups, got %d", len(tt.want), len(got))

			// Check each expected group
			for role, expectedSsets := range tt.want {
				gotSsets, exists := got[role]
				assert.True(t, exists, "Expected group for role %s not found", role)
				if !exists {
					continue
				}

				// Sort both slices for consistent comparison
				sort.Slice(expectedSsets, func(i, j int) bool {
					return expectedSsets[i].Name < expectedSsets[j].Name
				})
				sort.Slice(gotSsets, func(i, j int) bool {
					return gotSsets[i].Name < gotSsets[j].Name
				})

				assert.Equal(t, len(expectedSsets), len(gotSsets), "Group %s has wrong size", role)

				// Check if all StatefulSets in the group match
				for i := 0; i < len(expectedSsets); i++ {
					if i >= len(gotSsets) {
						t.Errorf("Missing StatefulSet at index %d in group %s", i, role)
						continue
					}

					assert.Equal(t, expectedSsets[i].Name, gotSsets[i].Name,
						"StatefulSet names do not match in group %s", role)
					assert.Equal(t, expectedSsets[i].Spec.Template.Labels, gotSsets[i].Spec.Template.Labels,
						"StatefulSet labels do not match in group %s", role)
				}
			}

			// Check if there are any unexpected groups
			for role := range got {
				_, exists := tt.want[role]
				assert.True(t, exists, "Unexpected group found: %s", role)
			}
		})
	}
}
