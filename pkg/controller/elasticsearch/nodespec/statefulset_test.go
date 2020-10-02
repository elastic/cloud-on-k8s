// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodespec

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	controllerscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
)

func Test_setVolumeClaimsControllerReference(t *testing.T) {
	controllerscheme.SetupScheme()
	varTrue := true
	varFalse := false
	es := esv1.Elasticsearch{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Elasticsearch",
			APIVersion: "elasticsearch.k8s.elastic.co/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es1",
			Namespace: "default",
			UID:       "ABCDEF",
		},
	}
	tests := []struct {
		name                   string
		persistentVolumeClaims []corev1.PersistentVolumeClaim
		existingClaims         []corev1.PersistentVolumeClaim
		wantClaims             []corev1.PersistentVolumeClaim
	}{
		{
			name: "should set the ownerRef when building a new StatefulSet",
			persistentVolumeClaims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"}},
			},
			existingClaims: nil,
			wantClaims: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         es.APIVersion,
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
			},
		},
		{
			name: "should set the ownerRef on user-provided claims when building a new StatefulSet",
			persistentVolumeClaims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "user-provided"}},
			},
			existingClaims: nil,
			wantClaims: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         es.APIVersion,
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "user-provided",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         es.APIVersion,
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
			},
		},
		{
			name: "should inherit existing claim ownerRefs that may have a different apiVersion",
			persistentVolumeClaims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "user-provided"}},
			},
			existingClaims: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
						OwnerReferences: []metav1.OwnerReference{
							{
								// claim already exists, with a different apiVersion
								APIVersion:         "elasticsearch.k8s.elastic.co/v1alpha1",
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "user-provided",
						OwnerReferences: []metav1.OwnerReference{
							{
								// claim already exists, with a different apiVersion
								APIVersion:         "elasticsearch.k8s.elastic.co/v1alpha1",
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
			},
			// existing claims should be preserved
			wantClaims: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1alpha1",
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "user-provided",
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "elasticsearch.k8s.elastic.co/v1alpha1",
								Kind:               es.Kind,
								Name:               es.Name,
								UID:                es.UID,
								Controller:         &varTrue,
								BlockOwnerDeletion: &varFalse,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := setVolumeClaimsControllerReference(tt.persistentVolumeClaims, tt.existingClaims, es)
			require.NoError(t, err)
			require.Equal(t, tt.wantClaims, got)
		})
	}
}
