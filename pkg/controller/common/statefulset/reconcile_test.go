// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	controllerscheme "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestWithTemplateHash(t *testing.T) {
	d := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stat",
			Namespace: "ns",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](2),
		},
	}

	withHash := WithTemplateHash(d)
	// the label should be set
	require.NotEmpty(t, withHash.Labels[hash.TemplateHashLabelName])
	// original object should be kept unmodified
	require.Empty(t, d.Labels)

	// label should stay the same if no spec change
	withSameHash := WithTemplateHash(d)
	require.Equal(t, withHash.Labels[hash.TemplateHashLabelName], withSameHash.Labels[hash.TemplateHashLabelName])

	// label should be different if the spec changed
	d.Spec.Replicas = ptr.To[int32](3)
	withDifferentHash := WithTemplateHash(d)
	require.NotEmpty(t, withDifferentHash.Labels[hash.TemplateHashLabelName])
	require.NotEqual(t, withHash.Labels[hash.TemplateHashLabelName], withDifferentHash.Labels[hash.TemplateHashLabelName])
}

func TestReconcile(t *testing.T) {
	controllerscheme.SetupScheme()
	k8sClient := k8s.NewFakeClient()
	expected := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stat",
			Namespace: "ns",
			Labels: map[string]string{
				"a": "b",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](2),
		},
	}
	owner := esv1.Elasticsearch{} // can be any type

	// should create a new StatefulSet
	reconciled, err := Reconcile(context.Background(), k8sClient, expected, &owner)
	require.NoError(t, err)
	// reconciled should match expected spec, and have the hash label set
	require.Equal(t, ptr.To[int32](2), reconciled.Spec.Replicas)
	require.Equal(t, "b", reconciled.Labels["a"])
	require.NotEmpty(t, reconciled.Labels[hash.TemplateHashLabelName])
	// resource should exist in the apiserver
	var retrieved appsv1.StatefulSet
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&expected), &retrieved)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &retrieved)

	// simulating a status update by the StatefulSet controller
	withStatusUpdate := retrieved
	withStatusUpdate.Status.Replicas = 2
	require.NoError(t, k8sClient.Status().Update(context.Background(), &withStatusUpdate))

	// reconciling the same should be a no-op
	reconciledAgain, err := Reconcile(context.Background(), k8sClient, expected, &owner)
	require.NoError(t, err)
	comparison.RequireEqual(t, &withStatusUpdate, &reconciledAgain)

	// update with a new spec
	expected.Spec.Replicas = ptr.To[int32](3)
	reconciled, err = Reconcile(context.Background(), k8sClient, expected, &owner)
	require.NoError(t, err)
	// both returned and retrieved should match that new spec
	require.Equal(t, 3, int(*reconciled.Spec.Replicas))
	// status update from earlier should still be unchanged
	require.Equal(t, 2, int(reconciled.Status.Replicas))
	require.NotEqual(t, reconciled.Labels[hash.TemplateHashLabelName], reconciledAgain.Labels[hash.TemplateHashLabelName])
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&expected), &retrieved)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &retrieved)
}
