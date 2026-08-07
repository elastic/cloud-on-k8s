// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package deployment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels/initcontainer"
	controllerscheme "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestWithTemplateHash(t *testing.T) {
	d := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep",
			Namespace: "ns",
		},
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
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep",
			Namespace: "ns",
			Labels: map[string]string{
				"a": "b",
			},
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

func TestReconcile_InitContainerImageNormalization(t *testing.T) {
	controllerscheme.SetupScheme()
	owner := esv1.Elasticsearch{}

	managedAnnotations := map[string]string{
		initcontainer.HashAnnotation: "managed:" + initcontainer.HashVersion,
	}
	initContainer := func(image string) corev1.Container {
		return corev1.Container{Name: initcontainer.ContainerName, Image: image, Command: []string{"/op", "wait-for-annotations"}}
	}
	mainContainer := func(image string) corev1.Container {
		return corev1.Container{Name: "main", Image: image}
	}

	for _, tc := range []struct {
		name                 string
		first                appsv1.Deployment
		second               appsv1.Deployment
		expectWorkloadUpdate bool
	}{
		{
			name: "init container image-only change does not update the workload",
			first: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Annotations: managedAnnotations},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{initContainer("eck-operator:1.0.0")},
							Containers:     []corev1.Container{mainContainer("app:1.0")},
						},
					},
				},
			},
			second: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Annotations: managedAnnotations},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{initContainer("eck-operator:1.0.1")},
							Containers:     []corev1.Container{mainContainer("app:1.0")},
						},
					},
				},
			},
			expectWorkloadUpdate: false,
		},
		{
			name: "main container image change updates the workload and picks up the new init container image",
			first: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep2", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Annotations: managedAnnotations},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{initContainer("eck-operator:1.0.0")},
							Containers:     []corev1.Container{mainContainer("app:1.0")},
						},
					},
				},
			},
			second: appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "dep2", Namespace: "ns"},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Annotations: managedAnnotations},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{initContainer("eck-operator:1.0.1")},
							Containers:     []corev1.Container{mainContainer("app:2.0")},
						},
					},
				},
			},
			expectWorkloadUpdate: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient()

			first, err := Reconcile(context.Background(), k8sClient, tc.first, &owner)
			require.NoError(t, err)
			firstHash := first.Labels[hash.TemplateHashLabelName]

			second, err := Reconcile(context.Background(), k8sClient, tc.second, &owner)
			require.NoError(t, err)
			secondHash := second.Labels[hash.TemplateHashLabelName]

			if tc.expectWorkloadUpdate {
				assert.NotEqual(t, firstHash, secondHash, "expected workload update but hash was unchanged")
				assert.Equal(t, "eck-operator:1.0.1", second.Spec.Template.Spec.InitContainers[0].Image,
					"updated workload should carry the new init container image")
			} else {
				assert.Equal(t, firstHash, secondHash, "expected no workload update but hash changed")
				assert.Equal(t, "eck-operator:1.0.0", second.Spec.Template.Spec.InitContainers[0].Image,
					"unchanged workload should keep the original init container image")
			}
		})
	}
}
