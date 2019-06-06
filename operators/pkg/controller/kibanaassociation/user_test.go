// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const userName = "default-kibana-foo-kibana-user"
const userSecretName = "kibana-foo-kibana-user"

var esFixture = estype.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "es-foo",
		Namespace: "default",
	},
}

var kibanaFixture = kbtype.Kibana{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "kibana-foo",
		Namespace: "default",
	},
	Spec: kbtype.KibanaSpec{
		ElasticsearchRef: commonv1alpha1.ObjectSelector{
			Name:      esFixture.Name,
			Namespace: esFixture.Namespace,
		},
	},
}

var t = true
var ownerRefFixture = metav1.OwnerReference{
	APIVersion:         "kibana.k8s.elastic.co/v1alpha1",
	Kind:               "Kibana",
	Name:               "foo",
	UID:                "",
	Controller:         &t,
	BlockOwnerDeletion: &t,
}

func setupScheme(t *testing.T) *runtime.Scheme {
	sc := scheme.Scheme
	if err := kbtype.SchemeBuilder.AddToScheme(sc); err != nil {
		assert.Fail(t, "failed to add Kibana types")
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
		kibana         kbtype.Kibana
		es             estype.Elasticsearch
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
				kibana:         kibanaFixture,
				es:             esFixture,
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
				kibana: kibanaFixture,
				es:     esFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				list := corev1.SecretList{}
				assert.NoError(t, c.List(&client.ListOptions{}, &list))
				assert.Equal(t, 3, len(list.Items))
				s := user.GetSecret(list, types.NamespacedName{Namespace: "other", Name: userSecretName})
				assert.NotNil(t, s)
				s = user.GetSecret(list, types.NamespacedName{Namespace: esFixture.Namespace, Name: userSecretName})
				assert.NotNil(t, s)
				password, passwordIsSet := s.Data[userName]
				assert.True(t, passwordIsSet)
				assert.NotEmpty(t, password)
				s = user.GetSecret(list, types.NamespacedName{Namespace: esFixture.Namespace, Name: userName}) // secret on the ES side
				user.ChecksUser(t, s, userName, []string{"kibana_system"})
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
				kibana: kibanaFixture,
				es:     esFixture,
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
						Name:      userName,
						Namespace: "default",
						Labels: map[string]string{
							AssociationLabelName: kibanaFixture.Name,
						},
					},
				}},
				kibana: kibanaFixture,
				es:     esFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var esUser corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: userName, Namespace: "default"}, &esUser))
				expectedLabels := map[string]string{
					AssociationLabelName:       kibanaFixture.Name,
					common.TypeLabelName:       user.UserType,
					label.ClusterNameLabelName: "es-foo",
				}
				for k, v := range expectedLabels {
					assert.Equal(t, v, esUser.Labels[k])
				}
			},
		},
		{
			name: "Reconcile avoids unnecessary updates",
			args: args{
				initialObjects: []runtime.Object{
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "default",
							Name:      userSecretName,
							Labels: map[string]string{
								kblabel.KibanaNameLabelName: kibanaFixture.Name,
								common.TypeLabelName:        kblabel.Type,
								AssociationLabelName:        kibanaFixture.Name,
							},
						},
						Data: map[string][]byte{
							kibanaUser: []byte("my-secret-pw"),
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userName,
							Namespace: "default",
							Labels: map[string]string{
								AssociationLabelName:       kibanaFixture.Name,
								AssociationLabelNamespace:  kibanaFixture.Namespace,
								common.TypeLabelName:       user.UserType,
								label.ClusterNameLabelName: esFixture.Name,
							},
						},
						Data: map[string][]byte{
							user.UserName:     []byte(userName),
							user.PasswordHash: []byte("$2a$10$mE3yo/AkZgR4eVW9kbA1TeIQ40Jv6WaWU494rx4C6EhLvuY0BSg4e"),
							user.UserRoles:    []byte(esuser.KibanaSystemUserBuiltinRole),
						},
					}},
				kibana: kibanaFixture,
				es:     esFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var userSecret corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: userName, Namespace: "default"}, &userSecret))
				require.Equal(t, "$2a$10$mE3yo/AkZgR4eVW9kbA1TeIQ40Jv6WaWU494rx4C6EhLvuY0BSg4e", string(userSecret.Data[user.PasswordHash]))
			},
		},
		{
			name: "Reconcile is namespace aware",
			args: args{
				kibana: kbtype.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kibana-foo",
						Namespace: "ns-2",
					},
					Spec: kbtype.KibanaSpec{
						ElasticsearchRef: commonv1alpha1.ObjectSelector{
							Name:      esFixture.Name,
							Namespace: esFixture.Namespace,
						},
					},
				},
				es: esFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				// user should be in ES namespace
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: "default",
					// name should include kibana namespace
					Name: "ns-2-kibana-foo-kibana-user",
				}, &corev1.Secret{}))
				// secret should be in Kibana namespace
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
			if err := reconcileEsUser(c, sc, tt.args.kibana, tt.args.es); (err != nil) != tt.wantErr {
				t.Errorf("reconcileEsUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.postCondition(c)
		})
	}
}
