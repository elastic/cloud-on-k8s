// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package deployment

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/hash"
	commonscheme "github.com/elastic/cloud-on-k8s/pkg/controller/common/scheme"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func TestWithTemplateHash(t *testing.T) {
	d := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep",
			Namespace: "ns",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(2),
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
	d.Spec.Replicas = pointer.Int32(3)
	withDifferentHash := WithTemplateHash(d)
	require.NotEmpty(t, withDifferentHash.Labels[hash.TemplateHashLabelName])
	require.NotEqual(t, withHash.Labels[hash.TemplateHashLabelName], withDifferentHash.Labels[hash.TemplateHashLabelName])
}

func TestReconcile(t *testing.T) {
	require.NoError(t, commonscheme.SetupScheme())
	k8sClient := k8s.WrappedFakeClient()
	expected := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dep",
			Namespace: "ns",
			Labels: map[string]string{
				"a": "b",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(2),
		},
	}
	owner := esv1.Elasticsearch{} // can be any type

	// should create a new deployment
	reconciled, err := Reconcile(k8sClient, expected, &owner)
	require.NoError(t, err)
	// reconciled should match expected spec, and have the hash label set
	require.Equal(t, pointer.Int32(2), reconciled.Spec.Replicas)
	require.Equal(t, "b", reconciled.Labels["a"])
	require.NotEmpty(t, reconciled.Labels[hash.TemplateHashLabelName])
	// resource should exist in the apiserver
	var retrieved appsv1.Deployment
	err = k8sClient.Get(k8s.ExtractNamespacedName(&expected), &retrieved)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &retrieved)

	// reconciling the same should be a no-op
	reconciledAgain, err := Reconcile(k8sClient, expected, &owner)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &reconciledAgain)

	// update with a new spec
	expected.Spec.Replicas = pointer.Int32(3)
	reconciled, err = Reconcile(k8sClient, expected, &owner)
	require.NoError(t, err)
	// both returned and retrieved should match that new spec
	require.Equal(t, pointer.Int32(3), reconciled.Spec.Replicas)
	require.NotEqual(t, reconciled.Labels[hash.TemplateHashLabelName], reconciledAgain.Labels[hash.TemplateHashLabelName])
	err = k8sClient.Get(k8s.ExtractNamespacedName(&expected), &retrieved)
	require.NoError(t, err)
	comparison.RequireEqual(t, &reconciled, &retrieved)
}
