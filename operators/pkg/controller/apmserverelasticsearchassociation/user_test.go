// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"testing"

	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/association"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const resourceNameFixture = "foo-elastic-internal-apm"

// associationFixture is a shared test fixture
var associationFixture = assoctype.ApmServerElasticsearchAssociation{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
	},
	Spec: assoctype.ApmServerElasticsearchAssociationSpec{
		Elasticsearch: assoctype.ObjectSelector{
			Name:      "es",
			Namespace: "default",
		},
		ApmServer: assoctype.ObjectSelector{
			Name:      "as",
			Namespace: "default",
		},
	},
}

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := assoctype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add assoc types")
	}
	if err := estype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Es types")
	}
	return sc
}

func Test_reconcileEsUser(t *testing.T) {
	sc := setupScheme(t)

	type args struct {
		initialObjects []runtime.Object
		assoc          assoctype.ApmServerElasticsearchAssociation
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		postCondition func(client k8s.Client)
	}{
		{
			name: "Happy path: should create a secret and a user CRD",
			args: args{
				initialObjects: nil,
				assoc:          associationFixture,
			},
			postCondition: func(c k8s.Client) {
				key := types.NamespacedName{
					Name:      resourceNameFixture,
					Namespace: "default",
				}
				assert.NoError(t, c.Get(key, &estype.User{}))
				assert.NoError(t, c.Get(key, &corev1.Secret{}))
			},
			wantErr: false,
		},
		{
			name: "Existing secret but different namespace: create new",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: "other",
					}}},
				assoc: associationFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				list := corev1.SecretList{}
				assert.NoError(t, c.List(&client.ListOptions{}, &list))
				assert.Equal(t, 2, len(list.Items))
				for _, s := range list.Items {
					assert.Equal(t, resourceNameFixture, s.Name)
				}
			},
		},
		{
			name: "Reconcile updates existing resources",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: "default",
					},
				}},
				assoc: associationFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var s corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: resourceNameFixture, Namespace: "default"}, &s))
				password, ok := s.Data[InternalApmServerUserName]
				assert.True(t, ok)
				assert.NotEmpty(t, password)
			},
		},
		{
			name: "Reconcile updates existing labels",
			args: args{
				initialObjects: []runtime.Object{&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: "default",
						Labels: map[string]string{
							association.AssociationLabelName: associationFixture.Name,
						},
					},
				}},
				assoc: associationFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var u estype.User
				assert.NoError(t, c.Get(types.NamespacedName{Name: resourceNameFixture, Namespace: "default"}, &u))
				expectedLabels := map[string]string{
					association.AssociationLabelName: associationFixture.Name,
					common.TypeLabelName:             label.Type,
					label.ClusterNameLabelName:       "es",
				}
				for k, v := range expectedLabels {
					assert.Equal(t, v, u.Labels[k])
				}
			},
		},
		{
			name: "Reconcile is namespace aware",
			args: args{
				assoc: assoctype.ApmServerElasticsearchAssociation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo",
						Namespace: "ns-1",
					},
					Spec: assoctype.ApmServerElasticsearchAssociationSpec{
						Elasticsearch: assoctype.ObjectSelector{
							Name:      "es",
							Namespace: "ns-1",
						},
						ApmServer: assoctype.ObjectSelector{
							Name:      "as",
							Namespace: "ns-2",
						},
					},
				},
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				// user CR should be in ES namespace
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: "ns-1",
					Name:      resourceNameFixture,
				}, &estype.User{}))
				// secret should be in Apm namespace
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: "ns-2",
					Name:      resourceNameFixture,
				}, &corev1.Secret{}))
			},
		},
	}
	for _, tt := range tests {
		c := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
		t.Run(tt.name, func(t *testing.T) {
			if err := reconcileEsUser(c, sc, tt.args.assoc); (err != nil) != tt.wantErr {
				t.Errorf("reconcileEsUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.postCondition(c)
		})
	}
}
