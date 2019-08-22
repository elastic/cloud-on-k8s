// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pdb

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcile(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))

	esMeta := metav1.ObjectMeta{
		Name:      "my-cluster",
		Namespace: "my-namespace",
	}
	esMeta.Labels = label.NewLabels(k8s.ExtractNamespacedName(&esMeta))

	intStrRef := func(i intstr.IntOrString) *intstr.IntOrString {
		return &i
	}

	type args struct {
		c  k8s.Client
		es v1alpha1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    *v1beta1.PodDisruptionBudget
		wantErr bool
	}{
		{
			name: "default",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				es: v1alpha1.Elasticsearch{
					ObjectMeta: esMeta,
				},
			},
			want: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name: name.DefaultPodDisruptionBudget(esMeta.Name), Namespace: esMeta.Namespace,
					Labels: label.NewLabels(k8s.ExtractNamespacedName(&esMeta)),
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							label.ClusterNameLabelName: esMeta.Name,
						},
					},
					MaxUnavailable: intStrRef(intstr.FromInt(1)),
				},
			},
		},
		{
			name: "custom pod disruption budget template",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				es: v1alpha1.Elasticsearch{
					ObjectMeta: esMeta,
					Spec: v1alpha1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1alpha1.PodDisruptionBudgetTemplate{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"foo": "bar",
								},
								Annotations: map[string]string{
									"annotation": "value",
								},
							},
							Spec: v1beta1.PodDisruptionBudgetSpec{
								MaxUnavailable: intStrRef(intstr.FromInt(42)),
								MinAvailable:   intStrRef(intstr.FromInt(123)),
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"foo": "bar",
									},
								},
							},
						},
					},
				},
			},
			want: &v1beta1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name: name.DefaultPodDisruptionBudget(esMeta.Name), Namespace: esMeta.Namespace,
					Labels: defaults.SetDefaultLabels(
						map[string]string{"foo": "bar"},
						label.NewLabels(k8s.ExtractNamespacedName(&esMeta)),
					),
					Annotations: map[string]string{
						"annotation": "value",
					},
				},
				Spec: v1beta1.PodDisruptionBudgetSpec{
					MaxUnavailable: intStrRef(intstr.FromInt(42)),
					MinAvailable:   intStrRef(intstr.FromInt(123)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
		{
			name: "pod disruption budget disabled: should not create",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient()),
				es: v1alpha1.Elasticsearch{
					ObjectMeta: esMeta,
					Spec: v1alpha1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1alpha1.PodDisruptionBudgetTemplate{},
					},
				},
			},
			want: nil,
		},
		{
			name: "pod disruption budget disabled: should delete",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&v1beta1.PodDisruptionBudget{
					ObjectMeta: metav1.ObjectMeta{
						Name: name.DefaultPodDisruptionBudget(esMeta.Name), Namespace: esMeta.Namespace,
					},
				})),
				es: v1alpha1.Elasticsearch{
					ObjectMeta: esMeta,
					Spec: v1alpha1.ElasticsearchSpec{
						PodDisruptionBudget: &commonv1alpha1.PodDisruptionBudgetTemplate{},
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Reconcile(tt.args.c, scheme.Scheme, tt.args.es)

			if (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil {
				var pdbs v1beta1.PodDisruptionBudgetList
				require.NoError(t, tt.args.c.List(&client.ListOptions{}, &pdbs))

				for _, pdb := range pdbs.Items {
					if tt.want != nil {
						tt.want.OwnerReferences = pdb.OwnerReferences
					}
					require.Equal(t, tt.want, &pdb)
				}

				if len(pdbs.Items) == 0 {
					require.Nil(t, tt.want)
				}
			}
		})
	}
}
