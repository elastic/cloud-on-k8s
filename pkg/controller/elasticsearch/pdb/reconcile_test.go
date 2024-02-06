// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	es_sset "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
)

func TestReconcile(t *testing.T) {
	defaultPDB := func() *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      esv1.DefaultPodDisruptionBudget("cluster"),
				Namespace: "ns",
				Labels:    map[string]string{label.ClusterNameLabelName: "cluster", commonv1.TypeLabelName: label.Type},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: intStrPtr(intstr.FromInt(3)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						label.ClusterNameLabelName: "cluster",
					},
				},
				MaxUnavailable: nil,
			},
		}
	}
	defaultEs := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}}
	type args struct {
		initObjs     []client.Object
		es           esv1.Elasticsearch
		statefulSets es_sset.StatefulSetList
	}
	tests := []struct {
		name    string
		args    args
		wantPDB *policyv1.PodDisruptionBudget
	}{
		{
			name: "no existing pdb: should create one",
			args: args{
				es:           defaultEs,
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: defaultPDB(),
		},
		{
			name: "pdb already exists: should remain unmodified",
			args: args{
				initObjs:     []client.Object{withHashLabel(withOwnerRef(defaultPDB(), defaultEs))},
				es:           defaultEs,
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: defaultPDB(),
		},
		{
			name: "pdb needs a MinAvailable update",
			args: args{
				initObjs:     []client.Object{defaultPDB()},
				es:           defaultEs,
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 5, Master: true, Data: true}.Build()},
			},
			wantPDB: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", commonv1.TypeLabelName: label.Type},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: intStrPtr(intstr.FromInt(5)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							label.ClusterNameLabelName: "cluster",
						},
					},
					MaxUnavailable: nil,
				},
			},
		},
		{
			name: "pdb disabled in the ES spec: should delete the existing one",
			args: args{
				initObjs: []client.Object{defaultPDB()},
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec:       esv1.ElasticsearchSpec{PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{}},
				},
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: nil,
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
			k8sClient := fake.NewClientBuilder().
				WithScheme(clientgoscheme.Scheme).
				WithRESTMapper(restMapper).
				WithObjects(tt.args.initObjs...).Build()

			err := Reconcile(context.Background(), k8sClient, tt.args.es, tt.args.statefulSets)
			require.NoError(t, err)
			pdbNsn := types.NamespacedName{Namespace: tt.args.es.Namespace, Name: esv1.DefaultPodDisruptionBudget(tt.args.es.Name)}
			var retrieved policyv1.PodDisruptionBudget
			err = k8sClient.Get(context.Background(), pdbNsn, &retrieved)
			if tt.wantPDB == nil {
				require.True(t, errors.IsNotFound(err))
			} else {
				// patch the PDB we want with ownerRef and hash label
				tt.wantPDB = withHashLabel(withOwnerRef(tt.wantPDB, tt.args.es))
				require.NoError(t, err)
				comparison.RequireEqual(t, tt.wantPDB, &retrieved)
			}
		})
	}
}

func withHashLabel(pdb *policyv1.PodDisruptionBudget) *policyv1.PodDisruptionBudget {
	pdb.Labels = hash.SetTemplateHashLabel(pdb.Labels, pdb)
	return pdb
}

func withOwnerRef(pdb *policyv1.PodDisruptionBudget, es esv1.Elasticsearch) *policyv1.PodDisruptionBudget {
	if err := controllerutil.SetControllerReference(&es, pdb, clientgoscheme.Scheme); err != nil {
		panic(err)
	}
	return pdb
}

func intStrPtr(intStr intstr.IntOrString) *intstr.IntOrString {
	return &intStr
}

func Test_expectedPDB(t *testing.T) {
	type args struct {
		es           esv1.Elasticsearch
		statefulSets es_sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want *policyv1.PodDisruptionBudget
	}{
		{
			name: "PDB disabled in the spec",
			args: args{
				es:           esv1.Elasticsearch{Spec: esv1.ElasticsearchSpec{PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{}}},
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: nil,
		},
		{
			name: "Build default PDB",
			args: args{
				es:           esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}},
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", commonv1.TypeLabelName: label.Type},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: intStrPtr(intstr.FromInt(3)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							label.ClusterNameLabelName: "cluster",
						},
					},
					MaxUnavailable: nil,
				},
			},
		},
		{
			name: "Inherit user-provided labels",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec: esv1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"a": "b", "c": "d"},
							}},
					},
				},
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{"a": "b", "c": "d", label.ClusterNameLabelName: "cluster", commonv1.TypeLabelName: label.Type},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: intStrPtr(intstr.FromInt(3)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							label.ClusterNameLabelName: "cluster",
						},
					},
					MaxUnavailable: nil,
				},
			},
		},
		{
			name: "Use user-provided PDB spec",
			args: args{
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec: esv1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1.PodDisruptionBudgetTemplate{
							Spec: policyv1.PodDisruptionBudgetSpec{MinAvailable: intStrPtr(intstr.FromInt(42))}},
					},
				},
				statefulSets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      esv1.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", commonv1.TypeLabelName: label.Type},
				},
				Spec: policyv1.PodDisruptionBudgetSpec{
					MinAvailable: intStrPtr(intstr.FromInt(42)),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want != nil {
				// set owner ref
				tt.want = withOwnerRef(tt.want, tt.args.es)
			}
			got, err := expectedPDB(tt.args.es, tt.args.statefulSets)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expectedPDB() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_allowedDisruptions(t *testing.T) {
	type args struct {
		es          esv1.Elasticsearch
		actualSsets es_sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want int32
	}{
		{
			name: "no health reported: no disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "yellow health: no disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchYellowHealth}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "red health: no disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchRedHealth}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "unknown health: no disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchUnknownHealth}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "green health: 1 disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "single-node cluster (not high-available): 1 disruption allowed",
			args: args{
				es:          esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				actualSsets: es_sset.StatefulSetList{sset.TestSset{Replicas: 1, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "green health but only 1 master: 0 disruption allowed",
			args: args{
				es: esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				actualSsets: es_sset.StatefulSetList{
					sset.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					sset.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 data node: 0 disruption allowed",
			args: args{
				es: esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				actualSsets: es_sset.StatefulSetList{
					sset.TestSset{Replicas: 3, Master: true, Data: false}.Build(),
					sset.TestSset{Replicas: 1, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 ingest node: 0 disruption allowed",
			args: args{
				es: esv1.Elasticsearch{Status: esv1.ElasticsearchStatus{Health: esv1.ElasticsearchGreenHealth}},
				actualSsets: es_sset.StatefulSetList{
					sset.TestSset{Replicas: 3, Master: true, Data: true, Ingest: false}.Build(),
					sset.TestSset{Replicas: 1, Ingest: true, Data: true}.Build(),
				},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allowedDisruptions(tt.args.es, tt.args.actualSsets); got != tt.want {
				t.Errorf("allowedDisruptions() = %v, want %v", got, tt.want)
			}
		})
	}
}
