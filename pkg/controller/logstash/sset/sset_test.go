// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package sset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// TestReconcile_PreservesExistingVCTs ensures that Logstash's Reconcile does not
// attempt to mutate the immutable VolumeClaimTemplates field of an existing
// StatefulSet. VCT label/metadata changes are handled by HandleVolumeExpansion
// (which updates PVCs directly), and storage resizes are handled by the
// recreate-annotation path. Any label-only diff on VCTs that reaches the
// apiserver would trigger a "Forbidden: updates to statefulset spec for fields
// other than ... volumeClaimTemplates ... are forbidden" error.
func TestReconcile_PreservesExistingVCTs(t *testing.T) {
	controllerscheme.SetupScheme()
	ls := lsv1alpha1.Logstash{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls", UID: types.UID("uid")},
		TypeMeta:   metav1.TypeMeta{Kind: lsv1alpha1.Kind},
	}

	existingVCT := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data"},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
			},
		},
	}
	// expectedVCT carries a label that does not exist on the cluster StatefulSet yet.
	expectedVCT := *existingVCT.DeepCopy()
	expectedVCT.ObjectMeta.Labels = map[string]string{"team": "search"}

	existingSset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ls.Namespace,
			Name:      "sset",
			Labels:    map[string]string{hash.TemplateHashLabelName: "old"},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             ptr.To[int32](3),
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{existingVCT},
		},
	}
	require.NoError(t, controllerutil.SetControllerReference(&ls, &existingSset, scheme.Scheme))

	expected := *existingSset.DeepCopy()
	expected.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{expectedVCT}
	// increment replicas so that NeedsUpdate returns true via the template-hash label.
	expected.Spec.Replicas = ptr.To[int32](4)
	expected.Labels = map[string]string{hash.TemplateHashLabelName: "new"}

	client := k8s.NewFakeClient(&existingSset)
	exp := expectations.NewExpectations(client, &appsv1.StatefulSet{})

	returned, err := Reconcile(context.Background(), client, expected, ls, exp)
	require.NoError(t, err)

	// the returned StatefulSet must keep the pre-existing VolumeClaimTemplates.
	require.Equal(t, []corev1.PersistentVolumeClaim{existingVCT}, returned.Spec.VolumeClaimTemplates)

	// the persisted StatefulSet must keep the pre-existing VolumeClaimTemplates too.
	var retrieved appsv1.StatefulSet
	require.NoError(t, client.Get(context.Background(), k8s.ExtractNamespacedName(&existingSset), &retrieved))
	require.Equal(t, []corev1.PersistentVolumeClaim{existingVCT}, retrieved.Spec.VolumeClaimTemplates)

	// non-VCT spec changes (replicas) should still be applied.
	require.Equal(t, ptr.To[int32](4), retrieved.Spec.Replicas)
	// and the new template-hash label should be applied.
	require.Equal(t, "new", retrieved.Labels[hash.TemplateHashLabelName])
}
