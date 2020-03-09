// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"context"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	esuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	userName       = "default-kibana-foo-kibana-user"
	userSecretName = "kibana-foo-kibana-user" // nolint
)

var esFixture = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "es-foo",
		Namespace: "default",
		UID:       "f8d564d9-885e-11e9-896d-08002703f062",
	},
}

var esRefFixture = metav1.OwnerReference{
	APIVersion:         "elasticsearch.k8s.elastic.co/v1",
	Kind:               "Elasticsearch",
	Name:               "es-foo",
	UID:                "f8d564d9-885e-11e9-896d-08002703f062",
	Controller:         &t,
	BlockOwnerDeletion: &t,
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

var t = true
var ownerRefFixture = metav1.OwnerReference{
	APIVersion:         "kibana.k8s.elastic.co/v1",
	Kind:               "Kibana",
	Name:               "foo",
	UID:                kibanaFixtureUID,
	Controller:         &t,
	BlockOwnerDeletion: &t,
}

func Test_deleteOrphanedResources(t *testing.T) {
	tests := []struct {
		name           string
		kibana         kbv1.Kibana
		es             esv1.Elasticsearch
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name: "Do not delete if there's no namespace in the ref",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ObjectSelector{ // ElasticsearchRef without a namespace
						Name: esFixture.Name,
						//Namespace: esFixture.Namespace, No namespace on purpose
					},
				},
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							esRefFixture,
						},
						Labels: map[string]string{
							AssociationLabelName: kibanaFixture.Name,
							common.TypeLabelName: esuser.AssociatedUserType,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c) // all objects must be exist
			},
			wantErr: false,
		},
		{
			name: "ES namespace has changed ",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbv1.KibanaSpec{
					ElasticsearchRef: commonv1.ObjectSelector{
						Name:      esFixture.Name,
						Namespace: "ns2", // Kibana does not reference the default namespace anymore
					},
				},
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: "default", // but we still have a user secret in default
						OwnerReferences: []metav1.OwnerReference{
							esRefFixture,
						},
						Labels: map[string]string{
							AssociationLabelName:      kibanaFixture.Name,
							AssociationLabelNamespace: kibanaFixture.Namespace,
							common.TypeLabelName:      esuser.AssociatedUserType,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				// user CR should be in ES namespace
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: esFixture.Namespace,
					Name:      userName,
				}, &corev1.Secret{}),
					"Previous user secret should have been removed")
			},
			wantErr: false,
		},
		{
			name:    "nothing to delete",
			kibana:  kbv1.Kibana{},
			wantErr: false,
		},
		{
			name:   "only valid objects",
			kibana: kibanaFixture,
			es:     esFixture,
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							esRefFixture,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c)
			},
			wantErr: false,
		},
		{
			name: "No more es ref in Kibana, orphan user & CA for previous es ref exist",
			kibana: kbv1.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
			},
			es: esFixture,
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: kibanaFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName: kibanaFixture.Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: kibanaFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName:      kibanaFixture.Name,
							AssociationLabelNamespace: kibanaFixture.Namespace,
						},
						OwnerReferences: []metav1.OwnerReference{
							esRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
						Namespace: kibanaFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName: kibanaFixture.Name,
						},
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				// This works even without labels because mock client currently ignores labels
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: kibanaFixture.Namespace,
					Name:      userName,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: kibanaFixture.Spec.ElasticsearchRef.Namespace,
					Name:      userSecretName,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: kibanaFixture.Spec.ElasticsearchRef.Namespace,
					Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
				}, &corev1.Secret{}))
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.initialObjects...)
			if err := deleteOrphanedResources(context.Background(), c, &tt.kibana); (err != nil) != tt.wantErr {
				t.Errorf("deleteOrphanedResources() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.postCondition != nil {
				tt.postCondition(c)
			}
		})
	}
}

func assertExpectObjectsExist(t *testing.T, c k8s.Client) {
	// user CR should be in ES namespace
	assert.NoError(t, c.Get(types.NamespacedName{
		Namespace: esFixture.Namespace,
		Name:      userName,
	}, &corev1.Secret{}))
	// user secret should be in Kibana namespace
	assert.NoError(t, c.Get(types.NamespacedName{
		Namespace: kibanaFixture.Namespace,
		Name:      userSecretName,
	}, &corev1.Secret{}))
	// ca secret should be in Kibana namespace
	assert.NoError(t, c.Get(types.NamespacedName{
		Namespace: kibanaFixture.Namespace,
		Name:      association.ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
	}, &corev1.Secret{}))
}
