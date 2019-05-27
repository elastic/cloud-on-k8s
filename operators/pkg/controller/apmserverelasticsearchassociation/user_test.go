// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	assoctype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/associations/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibanaassociation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	//resourceNameFixture = "as-elastic-internal-apm"
	userName       = "default-as-apm-user"
	userSecretName = "as-elastic-internal-apm"
)

// apmFixture is a shared test fixture
var apmFixture = apmtype.ApmServer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "as",
		Namespace: "default",
	},
	Spec: apmtype.ApmServerSpec{
		Output: apmtype.Output{
			Elasticsearch: apmtype.ElasticsearchOutput{
				ElasticsearchRef: &commonv1alpha1.ObjectSelector{
					Name:      "es",
					Namespace: "default",
				},
			},
		},
	},
}

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := assoctype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add assoc types")
	}
	if err := apmtype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add apm types")
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
		apm            apmtype.ApmServer
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
				apm:            apmFixture,
			},
			postCondition: func(c k8s.Client) {
				userKey := types.NamespacedName{
					Name:      userName,
					Namespace: "default",
				}
				assert.NoError(t, c.Get(userKey, &corev1.Secret{}))
				secretKey := types.NamespacedName{
					Name:      userSecretName,
					Namespace: "default",
				}
				assert.NoError(t, c.Get(secretKey, &corev1.Secret{}))
			},
			wantErr: false,
		},
		{
			name: "Existing secret but different namespace: create new",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: "other",
					}}},
				apm: apmFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				list := corev1.SecretList{}
				assert.NoError(t, c.List(&client.ListOptions{}, &list))
				assert.Equal(t, 3, len(list.Items))
				s := user.GetSecret(list, "other", userSecretName)
				assert.NotNil(t, s)
				s = user.GetSecret(list, apmFixture.Namespace, userSecretName)
				assert.NotNil(t, s)
				password, passwordIsSet := s.Data[userName]
				assert.True(t, passwordIsSet)
				assert.NotEmpty(t, password)
				s = user.GetSecret(list, apmFixture.Namespace, userName) // secret on the ES side
				user.ChecksUser(t, s, userName, []string{"superuser"})
			},
		},
		{
			name: "Reconcile updates existing resources",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: "default",
					},
				}},
				apm: apmFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var s corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: userSecretName, Namespace: "default"}, &s))
				password, ok := s.Data[userName]
				assert.True(t, ok)
				assert.NotEmpty(t, password)
			},
		},
		{
			name: "Reconcile updates existing labels",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: "default",
						Labels: map[string]string{
							kibanaassociation.AssociationLabelName: apmFixture.Name,
						},
					},
				}},
				apm: apmFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var u corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: userSecretName, Namespace: "default"}, &u))
				expectedLabels := map[string]string{
					kibanaassociation.AssociationLabelName: apmFixture.Name,
					common.TypeLabelName:                   label.Type,
					label.ClusterNameLabelName:             "es",
				}
				for k, v := range expectedLabels {
					assert.Equal(t, v, u.Labels[k])
				}
			},
		},
		{
			name: "Reconcile is namespace aware",
			args: args{
				apm: apmtype.ApmServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "as",
						Namespace: "ns-2",
					},
					Spec: apmtype.ApmServerSpec{
						Output: apmtype.Output{
							Elasticsearch: apmtype.ElasticsearchOutput{
								ElasticsearchRef: &commonv1alpha1.ObjectSelector{
									Name:      "es",
									Namespace: "ns-1",
								},
							},
						},
					},
				},
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				// user CR should be in ES namespace
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: "ns-1",
					Name:      "ns-2-as-apm-user",
				}, &corev1.Secret{}))
				// secret should be in Apm namespace
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: "ns-2",
					Name:      userSecretName,
				}, &corev1.Secret{}))
			},
		},
	}
	for _, tt := range tests {
		c := k8s.WrapClient(fake.NewFakeClient(tt.args.initialObjects...))
		t.Run(tt.name, func(t *testing.T) {
			if err := reconcileEsUser(c, sc, tt.args.apm); (err != nil) != tt.wantErr {
				t.Errorf("reconcileEsUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.postCondition(c)
		})
	}
}
