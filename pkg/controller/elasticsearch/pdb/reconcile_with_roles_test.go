// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"reflect"
	"slices"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	_ "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	_ "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

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
		stss     sset.StatefulSetList
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
			name: "no existing PDBs: should create role-specific PDBs and deduce roles from sts labels where sts is not listed as expected",
			args: args{
				es: defaultEs,
				stss: sset.StatefulSetList{
					ssetfixtures.TestSset{Name: "master1", Namespace: "ns", Master: true}.Build(),
					ssetfixtures.TestSset{Name: "data1", Namespace: "ns", Data: true}.Build(),
					// This additional sts is within the cluster, but not listed as "expected"
					// It's roles should be deduced from it's labels. This replicates the case
					// where an sts was recently deleted or renamed within the cluster.
					ssetfixtures.TestSset{Name: "frozen1", Namespace: "ns", DataFrozen: true}.Build(),
				},
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master1", 1, esv1.MasterRole).
					WithNodeSet("data1", 1, esv1.DataRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				// Unhealthy es cluster; 0 disruptions allowed
				rolePDB("cluster", "ns", esv1.MasterRole, []string{"master1"}, 0),
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data1"}, 0),
				rolePDB("cluster", "ns", esv1.DataFrozenRole, []string{"frozen1"}, 0),
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
			name: "no existing PDBs: should create role-specific PDBs with data roles grouped, allowing 1 disruption because single master node",
			args: args{
				es: *defaultHealthyES,
				builder: NewBuilder("cluster").
					WithNamespace("ns").
					WithNodeSet("master-data1", 1, esv1.MasterRole, esv1.DataRole).
					WithNodeSet("data2", 2, esv1.DataHotRole),
			},
			wantedPDBs: []*policyv1.PodDisruptionBudget{
				rolePDB("cluster", "ns", esv1.DataRole, []string{"data2", "master-data1"}, 1),
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
			c := fake.NewClientBuilder().
				WithScheme(clientgoscheme.Scheme).
				WithObjects(tt.args.initObjs...).
				Build()

			// Create metadata
			meta := metadata.Propagate(&tt.args.es, metadata.Metadata{Labels: tt.args.es.GetIdentityLabels()})

			statefulSets := tt.args.stss
			// allow for more control over the args to the function by
			// allowing for a custom stateful set list.
			if statefulSets == nil {
				statefulSets = tt.args.builder.GetStatefulSets()
			}

			err := reconcileRoleSpecificPDBs(context.Background(), c, tt.args.es, statefulSets, meta)
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

	defaultHealthyESWithPDBSpecified := defaultHealthyES.DeepCopy()
	defaultHealthyESWithPDBSpecified.Spec.PodDisruptionBudget = &commonv1.PodDisruptionBudgetTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"mykey": "myvalue",
			},
		},
	}

	defaultMeta := metadata.Metadata{
		Labels: map[string]string{
			"elasticsearch.k8s.elastic.co/cluster-name": "test-es",
		},
	}

	tests := []struct {
		name     string
		es       esv1.Elasticsearch
		builder  Builder
		meta     metadata.Metadata
		expected []*policyv1.PodDisruptionBudget
	}{
		{
			name:     "empty input",
			es:       *defaultHealthyES,
			builder:  NewBuilder("test-es").WithNamespace("ns").WithVersion("8.0.0"),
			meta:     defaultMeta,
			expected: []*policyv1.PodDisruptionBudget{},
		},
		{
			name: "single node cluster; role doesn't matter; 1 disruption with custom metadata",
			es:   *defaultHealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole),
			meta: defaultMeta.Merge(metadata.Metadata{Annotations: map[string]string{"custom": "annotation"}}),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "ns",
						Annotations: map[string]string{
							"custom": "annotation",
						},
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
			name: "single node cluster; role doesn't matter; 1 disruption with custom metadata in the pdb which is merged into the pdb",
			es:   *defaultHealthyESWithPDBSpecified,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole),
			meta: defaultMeta.Merge(metadata.Metadata{Annotations: map[string]string{"custom": "annotation"}}),
			expected: []*policyv1.PodDisruptionBudget{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es-es-default-master",
						Namespace: "ns",
						Annotations: map[string]string{
							"custom": "annotation",
						},
						Labels: map[string]string{
							"mykey":                    "myvalue",
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
			meta: defaultMeta,
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
			meta: defaultMeta,
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
			name: "master with a single node but unhealthy cluster should disallow disruptions",
			es:   defaultUnhealthyES,
			builder: NewBuilder("test-es").
				WithNamespace("ns").
				WithVersion("8.0.0").
				WithNodeSet("master1", 1, esv1.MasterRole),
			meta: defaultMeta,
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
			meta: defaultMeta,
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
			meta: defaultMeta,
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
			meta: defaultMeta,
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
			statefulSetList := tt.builder.GetStatefulSets()

			pdbs, err := expectedRolePDBs(context.Background(), tt.es, statefulSetList, tt.meta)
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
		es              esv1.Elasticsearch
		role            []esv1.NodeRole
		allStatefulSets sset.StatefulSetList
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
			name: "green health: 1 disruption allowed for data sts that is not HA",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role:            []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 1}.Build()},
			},
			want: 1,
		},
		{
			name: "green health: 1 disruption allowed for data sts that is HA",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role:            []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 2}.Build()},
			},
			want: 1,
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
			name: "single-node cluster (not high-available): 1 disruption allowed for master role when the cluster is green.",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 1, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "single-node cluster (not high-available): 1 disruption allowed for master role when the cluster is yellow.",
			args: args{
				es:              esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchYellowHealth}},
				role:            []esv1.NodeRole{esv1.MasterRole},
				allStatefulSets: sset.StatefulSetList{ssetfixtures.TestSset{Replicas: 1, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "green health with HA data tier: 1 disruption allowed for data role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
					ssetfixtures.TestSset{Replicas: 2, Ingest: true}.Build(),
				},
			},
			want: 1,
		},
		{
			name: "green health and only 1 data node: 1 disruption allowed for data role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Master: false, Data: true}.Build(),
				},
			},
			want: 1,
		},
		{
			name: "yellow health and only 1 data node: 0 disruption allowed for data role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchYellowHealth}},
				role: []esv1.NodeRole{esv1.DataRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 ingest node: 1 disruptions allowed for ingest role",
			args: args{
				es:   esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				role: []esv1.NodeRole{esv1.IngestRole},
				allStatefulSets: sset.StatefulSetList{
					ssetfixtures.TestSset{Replicas: 3, Master: true, Data: true, Ingest: false}.Build(),
					ssetfixtures.TestSset{Replicas: 1, Ingest: true, Data: true}.Build(),
					ssetfixtures.TestSset{Replicas: 1, DataFrozen: true}.Build(),
				},
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, role := range tt.args.role {
				if got := allowedDisruptionsForRole(context.Background(), tt.args.es, role, tt.args.allStatefulSets); got != tt.want {
					t.Errorf("allowedDisruptionsForRole() = %v, want %v for role: %s", got, tt.want, role)
				}
			}
		})
	}
}

func TestGetRolesFromStatefulSet(t *testing.T) {
	type args struct {
		statefulSetName string
		builder         Builder
		statefulSet     *appsv1.StatefulSet
		version         string
	}
	tests := []struct {
		name    string
		args    args
		want    []esv1.NodeRole
		wantErr bool
	}{
		{
			name: "unspecified roles (nil) - should represent all roles excluding coordinating",
			args: args{
				statefulSetName: "all-roles",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("all-roles", 3, "all_roles"),
				version: "8.0.0",
			},
			want: []esv1.NodeRole{
				esv1.MasterRole,
				esv1.DataRole,
				esv1.IngestRole,
				esv1.MLRole,
				esv1.TransformRole,
				esv1.RemoteClusterClientRole,
				esv1.DataHotRole,
				esv1.DataWarmRole,
				esv1.DataColdRole,
				esv1.DataContentRole,
				esv1.DataFrozenRole,
			},
			wantErr: false,
		},
		{
			name: "master only",
			args: args{
				statefulSetName: "master-only",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("master-only", 3, esv1.MasterRole),
				version: "8.0.0",
			},
			want:    []esv1.NodeRole{esv1.MasterRole},
			wantErr: false,
		},
		{
			name: "data only",
			args: args{
				statefulSetName: "data-only",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("data-only", 3, esv1.DataRole),
				version: "8.0.0",
			},
			want:    []esv1.NodeRole{esv1.DataRole},
			wantErr: false,
		},
		{
			name: "multiple roles",
			args: args{
				statefulSetName: "master-data",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("master-data", 3, esv1.MasterRole, esv1.DataRole),
				version: "8.0.0",
			},
			want:    []esv1.NodeRole{esv1.MasterRole, esv1.DataRole},
			wantErr: false,
		},
		{
			name: "coordinating node (empty roles slice)",
			args: args{
				statefulSetName: "coordinating",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("coordinating", 2, esv1.CoordinatingRole),
				version: "8.0.0",
			},
			want:    []esv1.NodeRole{esv1.CoordinatingRole},
			wantErr: false,
		},
		{
			name: "data tier roles",
			args: args{
				statefulSetName: "data-hot-warm",
				builder: NewBuilder("test-es").
					WithNamespace("ns").
					WithVersion("8.0.0").
					WithNodeSet("data-hot-warm", 3, esv1.DataHotRole, esv1.DataWarmRole),
				version: "8.0.0",
			},
			want:    []esv1.NodeRole{esv1.DataHotRole, esv1.DataWarmRole},
			wantErr: false,
		},
		{
			name: "sts with no labels returns an error",
			args: args{
				statefulSetName: "no-labels",
				statefulSet: &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-labels",
						Namespace: "ns",
					},
				},
				version: "8.0.0",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var statefulSet appsv1.StatefulSet
			var found bool
			// This allows overriding the statefulset for conditions such as error testing.
			if tt.args.statefulSet == nil {
				statefulSet, found = tt.args.builder.GetStatefulSets().GetByName(tt.args.statefulSetName)
			} else {
				statefulSet = *tt.args.statefulSet
				found = true
			}

			if !found && !tt.wantErr {
				t.Fatalf("StatefulSet %s not found in test fixtures", tt.args.statefulSetName)
			}

			got, err := getRolesFromStatefulSet(statefulSet)
			if err != nil && !tt.wantErr {
				t.Errorf("getRolesFromStatefulSet() error = %v", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getRolesForStatefulSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGroupBySharedRoles(t *testing.T) {
	tests := []struct {
		name    string
		builder Builder
		want    map[esv1.NodeRole][]appsv1.StatefulSet
	}{
		{
			name:    "empty statefulsets",
			builder: NewBuilder("test-es"),
			want:    map[esv1.NodeRole][]appsv1.StatefulSet{},
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stss := tt.builder.GetStatefulSets()

			got, err := groupBySharedRoles(stss)
			// There's only one path to an error here, which is a statefulset
			// with no labels, which currently this test doesn't have the ability to create.
			// There is a test in the underlying getRolesFromStatefulSet func that tests this condition.
			if err != nil {
				t.Errorf("groupBySharedRoles() error = %v", err)
				return
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
				for i := range expectedSsets {
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
