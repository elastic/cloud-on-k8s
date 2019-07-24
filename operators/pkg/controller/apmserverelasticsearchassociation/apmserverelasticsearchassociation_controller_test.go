// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var t = true
var ownerRefFixture = metav1.OwnerReference{
	APIVersion:         "apmserver.k8s.elastic.co/v1alpha1",
	Kind:               "ApmServer",
	Name:               "as",
	UID:                "",
	Controller:         &t,
	BlockOwnerDeletion: &t,
}

func Test_deleteOrphanedResources(t *testing.T) {
	s := setupScheme(t)
	tests := []struct {
		name           string
		args           apmtype.ApmServer
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name:    "nothing to delete",
			args:    apmtype.ApmServer{},
			wantErr: false,
		},
		{
			name: "only valid objects",
			args: apmFixture,
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: apmFixture.Namespace,
						OwnerReferences: []metav1.OwnerReference{
							ownerRefFixture,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esUserName,
						Namespace: apmFixture.Namespace,
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
			name: "Orphaned objects exist",
			args: apmtype.ApmServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "as",
					Namespace: "default",
				},
				Spec: apmtype.ApmServerSpec{},
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: apmFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName: apmFixture.Name,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esUserName,
						Namespace: apmFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName: apmFixture.Name,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				// This works even without labels because mock client currently ignores labels
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      userSecretName,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      esUserName,
				}, &corev1.Secret{}))

			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrapClient(fake.NewFakeClientWithScheme(s, tt.initialObjects...))
			if err := deleteOrphanedResources(c, tt.args); (err != nil) != tt.wantErr {
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
	require.NoError(t, c.Get(types.NamespacedName{
		Namespace: apmFixture.Namespace,
		Name:      userSecretName,
	}, &corev1.Secret{}))
	// secret should be in Kibana namespace
	require.NoError(t, c.Get(types.NamespacedName{
		Namespace: apmFixture.Namespace,
		Name:      esUserName,
	}, &corev1.Secret{}))
}
