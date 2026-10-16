// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestWithTemplateHash(t *testing.T) {
	d := appsv1.Deployment{
		Name:      "dep",
		Namespace: "ns",
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(2)),
		},
	}

	withHash := WithTemplateHash(d)
	// the label should be set
	require.NotEmpty(t, withHash.Labels[hash.TemplateHashLabelName])
	// original object should be kept unmodified
	require.Empty(t, d.Labels)

	// label should be consistent
	withSameHash := WithTemplateHash(d)
	require.Equal(t, withHash.Labels[hash.TemplateHashLabelName], withSameHash.Labels[hash.TemplateHashLabelName])

	// label should be the same if no spec changed
	withSameHash = WithTemplateHash(withSameHash)
	require.Equal(t, withHash.Labels[hash.TemplateHashLabelName], withSameHash.Labels[hash.TemplateHashLabelName])

	// label should be different if the spec changed
	d.Spec.Replicas = new(int32(3))
	withDifferentHash := WithTemplateHash(d)
	require.NotEmpty(t, withDifferentHash.Labels[hash.TemplateHashLabelName])
	require.NotEqual(t, withHash.Labels[hash.TemplateHashLabelName], withDifferentHash.Labels[hash.TemplateHashLabelName])
}

func TestReconcile(t *testing.T) {
	controllerscheme.SetupScheme()
	k8sClient := k8s.NewFakeClient()
	expected := appsv1.Deployment{
		Name:      "dep",
		Namespace: "ns",
		Labels: map[string]string{
			"a": "b",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(2)),
		},
	}
	owner := esv1.Elasticsearch{} // can be any type

	// should create a new deployment
	reconciled, err := Reconcile(context.Background(), k8sClient, expected, &owner)
	require.NoError(t, err)
	// reconciled should match expected spec, and have the hash label set
	require.Equal(t, new(int32(2)), reconciled.Spec.Replicas)
	require.Equal(t, "b", reconciled.Labels["a"])
	require.NotEmpty(t, reconciled.Labels[hash.TemplateHashLabelName])
	// resource should exist in the apiserver
	var retrieved appsv1.Deployment
	err = k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&expected), &retrieved)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &retrieved)

	// simulating a status update by the deployment controller
	withStatusUpdate := retrieved
	withStatusUpdate.Status.Replicas = 2
	require.NoError(t, k8sClient.Status().Update(context.Background(), &withStatusUpdate))

	// reconciling the same should be a no-op
	reconciledAgain, err := Reconcile(context.Background(), k8sClient, expected, &owner)
	require.NoError(t, err)
	comparison.RequireEqual(t, &withStatusUpdate, &reconciledAgain)

	// update with a new spec
	expected.Spec.Replicas = new(int32(3))
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

func TestReconcileNilReplicas(t *testing.T) {
	controllerscheme.SetupScheme()
	owner := esv1.Elasticsearch{}

	t.Run("nil replicas on create: API server defaults to 1", func(t *testing.T) {
		k8sClient := k8s.NewFakeClient()
		expected := appsv1.Deployment{
			Name:      "dep",
			Namespace: "ns",
			Spec:      appsv1.DeploymentSpec{Replicas: nil},
		}
		reconciled, err := Reconcile(context.Background(), k8sClient, expected, &owner)
		require.NoError(t, err)
		// Spec.Replicas stays nil in what ECK wrote; the fake client doesn't default it like
		// the real API server would, but the stored hash was computed with nil replicas.
		require.Nil(t, reconciled.Spec.Replicas)
	})

	t.Run("nil replicas preserves externally scaled value on subsequent reconcile", func(t *testing.T) {
		k8sClient := k8s.NewFakeClient()
		expected := appsv1.Deployment{
			Name:      "dep",
			Namespace: "ns",
			Spec:      appsv1.DeploymentSpec{Replicas: nil},
		}
		// First creation.
		_, err := Reconcile(context.Background(), k8sClient, expected, &owner)
		require.NoError(t, err)

		// Simulate HPA scaling the deployment to 5 replicas.
		var stored appsv1.Deployment
		require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&expected), &stored))
		stored.Spec.Replicas = new(int32(5))
		require.NoError(t, k8sClient.Update(context.Background(), &stored))

		// Reconcile again with nil replicas: ECK must not revert the HPA-managed count.
		reconciled, err := Reconcile(context.Background(), k8sClient, expected, &owner)
		require.NoError(t, err)
		require.Equal(t, int32(5), *reconciled.Spec.Replicas, "HPA-managed replica count must be preserved")
	})

	t.Run("non-nil replicas are still enforced", func(t *testing.T) {
		k8sClient := k8s.NewFakeClient()
		expected := appsv1.Deployment{
			Name:      "dep",
			Namespace: "ns",
			Spec:      appsv1.DeploymentSpec{Replicas: new(int32(2))},
		}
		_, err := Reconcile(context.Background(), k8sClient, expected, &owner)
		require.NoError(t, err)

		// Simulate something changing replicas externally.
		var stored appsv1.Deployment
		require.NoError(t, k8sClient.Get(context.Background(), k8s.ExtractNamespacedName(&expected), &stored))
		stored.Spec.Replicas = new(int32(99))
		stored.Labels = nil // clear hash so NeedsUpdate sees a diff
		require.NoError(t, k8sClient.Update(context.Background(), &stored))

		// Reconcile with non-nil replicas: ECK must restore the desired count.
		reconciled, err := Reconcile(context.Background(), k8sClient, expected, &owner)
		require.NoError(t, err)
		require.Equal(t, int32(2), *reconciled.Spec.Replicas, "ECK-managed replica count must be enforced")
	})
}

func TestIsSteady(t *testing.T) {
	tests := []struct {
		name string
		d    appsv1.Deployment
		want bool
	}{
		{
			name: "all replicas updated and available",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: new(int32(3))},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 3, UpdatedReplicas: 3, AvailableReplicas: 3},
			},
			want: true,
		},
		{
			name: "nil replicas defaults to 1 and is steady",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
			},
			want: true,
		},
		{
			name: "nil replicas defaults to 1 and pod not available",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 1},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1},
			},
			want: false,
		},
		{
			name: "generation not yet observed",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 3},
				Spec:       appsv1.DeploymentSpec{Replicas: new(int32(1))},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, AvailableReplicas: 1},
			},
			want: false,
		},
		{
			name: "rolling update in progress: old replicas still present",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: new(int32(2))},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 3, UpdatedReplicas: 2, AvailableReplicas: 2},
			},
			want: false,
		},
		{
			name: "updated replicas not all available",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: new(int32(2))},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 1},
			},
			want: false,
		},
		{
			name: "fewer replicas than spec",
			d: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Generation: 2},
				Spec:       appsv1.DeploymentSpec{Replicas: new(int32(3))},
				Status:     appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsSteady(&tt.d))
		})
	}
}
