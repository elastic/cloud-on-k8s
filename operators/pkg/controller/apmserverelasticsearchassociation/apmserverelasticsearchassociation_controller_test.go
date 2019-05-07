// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"testing"

	assoctype "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/kibanaassociation"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
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
		args           assoctype.ApmServerElasticsearchAssociation
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name:    "nothing to delete",
			args:    assoctype.ApmServerElasticsearchAssociation{},
			wantErr: false,
		},
		{
			name: "only valid objects",
			args: associationFixture,
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: associationFixture.Namespace,
					},
				},
				&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: associationFixture.Namespace,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c)
			},
			wantErr: false,
		},
		{
			name: "Orpaned objects exist",
			args: associationFixture,
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: associationFixture.Namespace,
						Labels: map[string]string{
							kibanaassociation.AssociationLabelName: associationFixture.Name,
						},
					},
				},
				&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: associationFixture.Namespace,
						Labels: map[string]string{
							kibanaassociation.AssociationLabelName: associationFixture.Name,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: "other-ns",
						Labels: map[string]string{
							kibanaassociation.AssociationLabelName: associationFixture.Name,
						},
					},
				},
				&estype.User{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceNameFixture,
						Namespace: "other-ns",
						Labels: map[string]string{
							kibanaassociation.AssociationLabelName: associationFixture.Name,
						},
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assertExpectObjectsExist(t, c)
				// This works even without labels because mock client currently ignores labels
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      resourceNameFixture,
				}, &estype.User{}))
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      resourceNameFixture,
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
	assert.NoError(t, c.Get(types.NamespacedName{
		Namespace: associationFixture.Namespace,
		Name:      resourceNameFixture,
	}, &estype.User{}))
	// secret should be in Kibana namespace
	assert.NoError(t, c.Get(types.NamespacedName{
		Namespace: associationFixture.Namespace,
		Name:      resourceNameFixture,
	}, &corev1.Secret{}))
}
