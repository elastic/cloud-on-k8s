// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

func Test_deleteOrphanedResources(t *testing.T) {
	s := setupScheme(t)
	tests := []struct {
		name           string
		kibana         kbtype.Kibana
		es             types.NamespacedName
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
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
				&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
						Namespace: kibanaFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kibana-foo",
					Namespace: "default",
				},
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
				&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userName,
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
				}, &estype.User{}))
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
	}, &estype.User{}))
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
