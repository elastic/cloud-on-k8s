// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackconfig"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
		es                     esv1.Elasticsearch
		persistentVolumeClaims []corev1.PersistentVolumeClaim
		existingClaims         []corev1.PersistentVolumeClaim
		wantClaims             []corev1.PersistentVolumeClaim
	}{
		{
			name: "should not set the ownerRef when building a new StatefulSet",
			persistentVolumeClaims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-data"}},
			},
			existingClaims: nil,
			wantClaims: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "elasticsearch-data",
					},
				},
			},
		},
		{
			name: "should inherit existing claim ownerRefs for backwards compatibility (that may also have a different apiVersion)",
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
			got := preserveExistingVolumeClaimsOwnerRefs(tt.persistentVolumeClaims, tt.existingClaims)
			require.Equal(t, tt.wantClaims, got)
		})
	}
}

func Test_BuildStatefulSet_PVCRetentionPolicy(t *testing.T) {
	tests := []struct {
		name            string
		deletePolicy    esv1.VolumeClaimDeletePolicy
		wantWhenDeleted appsv1.PersistentVolumeClaimRetentionPolicyType
		wantWhenScaled  appsv1.PersistentVolumeClaimRetentionPolicyType
	}{
		{
			name:            "DeleteOnScaledownAndClusterDeletion sets whenDeleted=Delete",
			deletePolicy:    esv1.DeleteOnScaledownAndClusterDeletionPolicy,
			wantWhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			wantWhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		},
		{
			name:            "DeleteOnScaledownOnly sets whenDeleted=Retain",
			deletePolicy:    esv1.DeleteOnScaledownOnlyPolicy,
			wantWhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
			wantWhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		},
		{
			name:            "empty policy (default) sets whenDeleted=Delete",
			deletePolicy:    "",
			wantWhenDeleted: appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			wantWhenScaled:  appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		},
	}

	nodeSet := esv1.NodeSet{
		Name:  "default",
		Count: 1,
		Config: &commonv1.Config{
			Data: map[string]any{"node.roles": []string{"master", "data"}},
		},
		PodTemplate: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: esv1.ElasticsearchContainerName}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esObj := newEsSampleBuilder().withVersion("8.14.0").build()
			esObj.Spec.NodeSets = []esv1.NodeSet{nodeSet}
			esObj.Spec.VolumeClaimDeletePolicy = tt.deletePolicy

			client := k8s.NewFakeClient(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: esObj.Namespace, Name: esv1.ScriptsConfigMap(esObj.Name)},
			})

			ver, err := version.Parse(esObj.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(
				esObj.Name, ver, corev1.IPv4Protocol, esObj.Spec.HTTP,
				*nodeSet.Config, nil, false, false, false, false,
			)
			require.NoError(t, err)

			sts, err := BuildStatefulSet(
				t.Context(), client, esObj, nodeSet, cfg,
				nil, nil, false, stackconfig.PolicyConfig{}, metadata.Metadata{}, "", false,
			)
			require.NoError(t, err)
			require.NotNil(t, sts.Spec.PersistentVolumeClaimRetentionPolicy,
				"PersistentVolumeClaimRetentionPolicy must be set")
			require.Equal(t, tt.wantWhenDeleted, sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted)
			require.Equal(t, tt.wantWhenScaled, sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled)
		})
	}
}
