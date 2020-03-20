// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"context"
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

const (
	esUserName     = "default-as-apm-user"
	userSecretName = "as-elastic-internal-apm"
)

// apmFixture is a shared test fixture
var apmFixture = apmv1.ApmServer{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "as",
		Namespace: "default",
	},
	Spec: apmv1.ApmServerSpec{
		ElasticsearchRef: commonv1.ObjectSelector{
			Name:      "es",
			Namespace: "default",
		},
	},
}

func Test_deleteOrphanedResources(t *testing.T) {
	tests := []struct {
		name           string
		args           apmv1.ApmServer
		initialObjects []runtime.Object
		postCondition  func(c k8s.Client)
		wantErr        bool
	}{
		{
			name:    "nothing to delete",
			args:    apmv1.ApmServer{},
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
						Labels: map[string]string{
							AssociationLabelName:      apmFixture.Name,
							AssociationLabelNamespace: apmFixture.Namespace,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esUserName,
						Namespace: apmFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName:      apmFixture.Name,
							AssociationLabelNamespace: apmFixture.Namespace,
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
			args: apmv1.ApmServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "as",
					Namespace: "default",
				},
				Spec: apmv1.ApmServerSpec{},
			},
			initialObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      userSecretName,
						Namespace: apmFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName:      apmFixture.Name,
							AssociationLabelNamespace: apmFixture.Namespace,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      esUserName,
						Namespace: apmFixture.Namespace,
						Labels: map[string]string{
							AssociationLabelName:      apmFixture.Name,
							AssociationLabelNamespace: apmFixture.Namespace,
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "should-not-be-deleted",
						Namespace: apmFixture.Namespace,
					},
				},
			},
			postCondition: func(c k8s.Client) {
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      userSecretName,
				}, &corev1.Secret{}))
				assert.Error(t, c.Get(types.NamespacedName{
					Namespace: "other-ns",
					Name:      esUserName,
				}, &corev1.Secret{}))
				assert.NoError(t, c.Get(types.NamespacedName{
					Namespace: apmFixture.Namespace,
					Name:      "should-not-be-deleted",
				}, &corev1.Secret{}))
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient(tt.initialObjects...)
			if err := deleteOrphanedResources(context.Background(), c, &tt.args); (err != nil) != tt.wantErr {
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
