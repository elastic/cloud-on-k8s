// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"testing"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_deleteOrphanedResources(t *testing.T) {
	s := setupScheme(t)
	tests := []struct {
		name           string
		kibana         kbtype.Kibana
		es             v1alpha1.Elasticsearch
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name: "Do not delete if there's no namespace in the ref",
			kibana: kbtype.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbtype.KibanaSpec{
					ElasticsearchRef: commonv1alpha1.ObjectSelector{ // ElasticsearchRef without a namespace
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
						Name:      CACertSecretName(kibanaFixture.Name),
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
							common.TypeLabelName: user.UserType,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c)
				// user CR should be in ES namespace
				/*assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: esFixture.Namespace,
					Name:      userName,
				}, &corev1.Secret{}),
					"Elasticsearch should not be deleted")*/
			},
			wantErr: false,
		},
		{
			name: "ES namespace has changed ",
			kibana: kbtype.Kibana{
				ObjectMeta: kibanaFixtureObjectMeta,
				Spec: kbtype.KibanaSpec{
					ElasticsearchRef: commonv1alpha1.ObjectSelector{
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
						Name:      CACertSecretName(kibanaFixture.Name),
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
							common.TypeLabelName:      user.UserType,
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
			kibana:  kbtype.Kibana{},
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
						Name:      CACertSecretName(kibanaFixture.Name),
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
			kibana: kbtype.Kibana{
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
						Name:      CACertSecretName(kibanaFixture.Name),
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
					Name:      CACertSecretName(kibanaFixture.Name),
				}, &corev1.Secret{}))
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrapClient(fake.NewFakeClientWithScheme(s, tt.initialObjects...))
			if err := deleteOrphanedResources(c, tt.kibana); (err != nil) != tt.wantErr {
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
		Name:      CACertSecretName(kibanaFixture.Name),
	}, &corev1.Secret{}))
}
