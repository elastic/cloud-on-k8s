// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	kblabel "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const (
	userName                  = "default-kibana-foo-kibana-user"
	userSecretName            = "kibana-foo-kibana-user" // nolint
	associationLabelName      = "association.k8s.elastic.co/name"
	associationLabelNamespace = "association.k8s.elastic.co/namespace"
)

var esFixture = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "es-foo",
		Namespace: "default",
		UID:       "f8d564d9-885e-11e9-896d-08002703f062",
	},
}

var kibanaFixtureUID types.UID = "82257b19-8862-11e9-896d-08002703f062"

var kibanaFixtureObjectMeta = metav1.ObjectMeta{
	Name:      "kibana-foo",
	Namespace: "default",
	UID:       kibanaFixtureUID,
}

var kibanaFixture = kbv1.Kibana{
	ObjectMeta: kibanaFixtureObjectMeta,
	Spec: kbv1.KibanaSpec{
		ElasticsearchRef: commonv1.ObjectSelector{
			Name:      esFixture.Name,
			Namespace: esFixture.Namespace,
		},
	},
}

func Test_reconcileEsUser(t *testing.T) {
	type args struct {
		initialObjects []runtime.Object
		kibana         kbv1.Kibana
		es             esv1.Elasticsearch
	}
	tests := []struct {
		name          string
		args          args
		wantErr       bool
		postCondition func(client k8s.Client)
	}{
		{
			name: "Reconcile updates existing labels",
			args: args{
				initialObjects: []runtime.Object{&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: "default",
						Labels: map[string]string{
							associationLabelName: kibanaFixture.Name,
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
					associationLabelName:       kibanaFixture.Name,
					common.TypeLabelName:       esuser.AssociatedUserType,
					label.ClusterNameLabelName: "es-foo",
				}
				for k, v := range expectedLabels {
					assert.Equal(t, v, esUser.Labels[k])
				}
			},
		},
		{
			name: "Happy path: should create two secrets",
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
				assert.NoError(t, c.List(&list))
				assert.Equal(t, 3, len(list.Items))
				s := GetSecret(list, types.NamespacedName{Namespace: "other", Name: userSecretName})
				assert.NotNil(t, s)
				s = GetSecret(list, types.NamespacedName{Namespace: esFixture.Namespace, Name: userSecretName})
				assert.NotNil(t, s)
				password, passwordIsSet := s.Data[userName]
				assert.True(t, passwordIsSet)
				assert.NotEmpty(t, password)
				s = GetSecret(list, types.NamespacedName{Namespace: esFixture.Namespace, Name: userName}) // secret on the ES side
				ChecksUser(t, s, userName, []string{"kibana_system"})
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
								associationLabelName:        kibanaFixture.Name,
								associationLabelNamespace:   kibanaFixture.Namespace,
							},
						},
						Data: map[string][]byte{
							userName: []byte("my-secret-pw"),
						},
					},
					&corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      userName,
							Namespace: "default",
							Labels: map[string]string{
								associationLabelName:       kibanaFixture.Name,
								associationLabelNamespace:  kibanaFixture.Namespace,
								common.TypeLabelName:       esuser.AssociatedUserType,
								label.ClusterNameLabelName: esFixture.Name,
							},
						},
						Data: map[string][]byte{
							esuser.UserNameField:     []byte(userName),
							esuser.PasswordHashField: []byte("$2a$10$mE3yo/AkZgR4eVW9kbA1TeIQ40Jv6WaWU494rx4C6EhLvuY0BSg4e"),
							esuser.UserRolesField:    []byte("kibana_system"),
						},
					}},
				kibana: kibanaFixture,
				es:     esFixture,
			},
			wantErr: false,
			postCondition: func(c k8s.Client) {
				var userSecret corev1.Secret
				assert.NoError(t, c.Get(types.NamespacedName{Name: userName, Namespace: "default"}, &userSecret))
				require.Equal(t, "$2a$10$mE3yo/AkZgR4eVW9kbA1TeIQ40Jv6WaWU494rx4C6EhLvuY0BSg4e", string(userSecret.Data[esuser.PasswordHashField]))
			},
		},
		{
			name: "Reconcile is namespace aware",
			args: args{
				kibana: kbv1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kibana-foo",
						Namespace: "ns-2",
					},
					Spec: kbv1.KibanaSpec{
						ElasticsearchRef: commonv1.ObjectSelector{
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
		c := k8s.WrappedFakeClient(tt.args.initialObjects...)
		t.Run(tt.name, func(t *testing.T) {
			if err := ReconcileEsUser(
				context.Background(),
				c,
				scheme.Scheme,
				&tt.args.kibana,
				map[string]string{
					associationLabelName:      tt.args.kibana.Name,
					associationLabelNamespace: tt.args.kibana.Namespace,
				},
				"kibana_system",
				"kibana-user",
				tt.args.es,
			); (err != nil) != tt.wantErr {
				t.Errorf("reconcileEsUser() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.postCondition(c)
		})
	}
}

// ChecksUser checks that a secret contains the required fields expected by the user reconciler.
func ChecksUser(t *testing.T, secret *corev1.Secret, expectedUsername string, expectedRoles []string) {
	assert.NotNil(t, secret)
	currentUsername, ok := secret.Data["name"]
	assert.True(t, ok)
	assert.Equal(t, expectedUsername, string(currentUsername))
	passwordHash, ok := secret.Data["passwordHash"]
	assert.True(t, ok)
	assert.NotEmpty(t, passwordHash)
	currentRoles, ok := secret.Data["userRoles"]
	assert.True(t, ok)
	assert.ElementsMatch(t, expectedRoles, strings.Split(string(currentRoles), ","))
}

// GetSecret gets the first secret in a list that matches the namespace and the name.
func GetSecret(list corev1.SecretList, namespacedName types.NamespacedName) *corev1.Secret {
	for _, secret := range list.Items {
		if secret.Namespace == namespacedName.Namespace && secret.Name == namespacedName.Name {
			return &secret
		}
	}
	return nil
}
