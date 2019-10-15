// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pdb

import (
	"reflect"
	"testing"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	// "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestReconcile(t *testing.T) {
	require.NoError(t, esv1beta1.AddToScheme(scheme.Scheme))
	defaultPDB := func() *v1beta1.PodDisruptionBudget {
		return &v1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.DefaultPodDisruptionBudget("cluster"),
				Namespace: "ns",
				Labels:    map[string]string{label.ClusterNameLabelName: "cluster", common.TypeLabelName: label.Type},
			},
			Spec: v1beta1.PodDisruptionBudgetSpec{
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
	defaultEs := esv1beta1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}}
	type args struct {
		k8sClient    k8s.Client
		es           esv1beta1.Elasticsearch
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name    string
		args    args
		wantPDB *v1beta1.PodDisruptionBudget
	}{
		{
			name: "no existing pdb: should create one",
			args: args{
				k8sClient:    k8s.WrapClient(fake.NewFakeClient()),
				es:           defaultEs,
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: defaultPDB(),
		},
		{
			name: "pdb already exists: should remain unmodified",
			args: args{
				k8sClient:    k8s.WrapClient(fake.NewFakeClient(withHashLabel(withOwnerRef(defaultPDB(), defaultEs)))),
				es:           defaultEs,
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: defaultPDB(),
		},
		{
			name: "pdb needs a MinAvailable update",
			args: args{
				k8sClient:    k8s.WrapClient(fake.NewFakeClient(defaultPDB())),
				es:           defaultEs,
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 5, Master: true, Data: true}.Build()},
			},
			wantPDB: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", common.TypeLabelName: label.Type},
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
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
				k8sClient: k8s.WrapClient(fake.NewFakeClient(defaultPDB())),
				es: esv1beta1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec:       esv1beta1.ElasticsearchSpec{PodDisruptionBudget: &commonv1beta1.PodDisruptionBudgetTemplate{}},
				},
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			wantPDB: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Reconcile(tt.args.k8sClient, scheme.Scheme, tt.args.es, tt.args.statefulSets)
			require.NoError(t, err)
			pdbNsn := types.NamespacedName{Namespace: tt.args.es.Namespace, Name: name.DefaultPodDisruptionBudget(tt.args.es.Name)}
			var retrieved v1beta1.PodDisruptionBudget
			err = tt.args.k8sClient.Get(pdbNsn, &retrieved)
			if tt.wantPDB == nil {
				require.True(t, errors.IsNotFound(err))
			} else {
				// patch the PDB we want with ownerRef and hash label
				tt.wantPDB = withHashLabel(withOwnerRef(tt.wantPDB, tt.args.es))
				require.NoError(t, err)
				require.Equal(t, tt.wantPDB, &retrieved)
			}
		})
	}
}

func withHashLabel(pdb *v1beta1.PodDisruptionBudget) *v1beta1.PodDisruptionBudget {
	pdb.Labels = hash.SetTemplateHashLabel(pdb.Labels, pdb)
	return pdb
}

func withOwnerRef(pdb *v1beta1.PodDisruptionBudget, es esv1beta1.Elasticsearch) *v1beta1.PodDisruptionBudget {
	if err := controllerutil.SetControllerReference(&es, pdb, scheme.Scheme); err != nil {
		panic(err)
	}
	return pdb
}

func intStrPtr(intStr intstr.IntOrString) *intstr.IntOrString {
	return &intStr
}

func Test_expectedPDB(t *testing.T) {
	require.NoError(t, esv1beta1.AddToScheme(scheme.Scheme))
	type args struct {
		es           esv1beta1.Elasticsearch
		statefulSets sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want *v1beta1.PodDisruptionBudget
	}{
		{
			name: "PDB disabled in the spec",
			args: args{
				es:           esv1beta1.Elasticsearch{Spec: esv1beta1.ElasticsearchSpec{PodDisruptionBudget: &commonv1beta1.PodDisruptionBudgetTemplate{}}},
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: nil,
		},
		{
			name: "Build default PDB",
			args: args{
				es:           esv1beta1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}},
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", common.TypeLabelName: label.Type},
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
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
				es: esv1beta1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec: esv1beta1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1beta1.PodDisruptionBudgetTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"a": "b", "c": "d"},
							}},
					},
				},
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{"a": "b", "c": "d", label.ClusterNameLabelName: "cluster", common.TypeLabelName: label.Type},
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
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
				es: esv1beta1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
					Spec: esv1beta1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1beta1.PodDisruptionBudgetTemplate{
							Spec: v1beta1.PodDisruptionBudgetSpec{MinAvailable: intStrPtr(intstr.FromInt(42))}},
					},
				},
				statefulSets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name.DefaultPodDisruptionBudget("cluster"),
					Namespace: "ns",
					Labels:    map[string]string{label.ClusterNameLabelName: "cluster", common.TypeLabelName: label.Type},
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
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
			got, err := expectedPDB(tt.args.es, tt.args.statefulSets, scheme.Scheme)
			require.NoError(t, err)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expectedPDB() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_allowedDisruptions(t *testing.T) {
	type args struct {
		es          esv1beta1.Elasticsearch
		actualSsets sset.StatefulSetList
	}
	tests := []struct {
		name string
		args args
		want int32
	}{
		{
			name: "no health reported: no disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "yellow health: no disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchYellowHealth}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 3}.Build()},
			},
			want: 0,
		},
		{
			name: "red health: no disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchRedHealth}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "unknown health: no disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchUnknownHealth}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "green health: 1 disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchGreenHealth}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 3, Master: true, Data: true}.Build()},
			},
			want: 1,
		},
		{
			name: "green health but single-node cluster: 0 disruption allowed",
			args: args{
				es:          esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchGreenHealth}},
				actualSsets: sset.StatefulSetList{sset.TestSset{Replicas: 1, Master: true, Data: true}.Build()},
			},
			want: 0,
		},
		{
			name: "green health but only 1 master: 0 disruption allowed",
			args: args{
				es: esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchGreenHealth}},
				actualSsets: sset.StatefulSetList{
					sset.TestSset{Replicas: 1, Master: true, Data: false}.Build(),
					sset.TestSset{Replicas: 3, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 data node: 0 disruption allowed",
			args: args{
				es: esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchGreenHealth}},
				actualSsets: sset.StatefulSetList{
					sset.TestSset{Replicas: 3, Master: true, Data: false}.Build(),
					sset.TestSset{Replicas: 1, Master: false, Data: true}.Build(),
				},
			},
			want: 0,
		},
		{
			name: "green health but only 1 ingest node: 0 disruption allowed",
			args: args{
				es: esv1beta1.Elasticsearch{Status: esv1beta1.ElasticsearchStatus{Health: esv1beta1.ElasticsearchGreenHealth}},
				actualSsets: sset.StatefulSetList{
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
