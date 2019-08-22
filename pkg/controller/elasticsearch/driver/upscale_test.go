// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestHandleUpscaleAndSpecChanges(t *testing.T) {
	require.NoError(t, v1alpha1.AddToScheme(scheme.Scheme))
	k8sClient := k8s.WrapClient(fake.NewFakeClient())
	es := v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"}}
	expectedResources := nodespec.ResourcesList{
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: common.Int32(3),
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset1",
				},
			},
			Config: settings.CanonicalConfig{},
		},
		{
			StatefulSet: appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset2",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: common.Int32(4),
				},
			},
			HeadlessService: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "sset2",
				},
			},
			Config: settings.CanonicalConfig{},
		},
	}

	// when no StatefulSets already exists
	actualStatefulSets := sset.StatefulSetList{}
	err := HandleUpscaleAndSpecChanges(k8sClient, es, scheme.Scheme, expectedResources, actualStatefulSets)
	require.NoError(t, err)
	// StatefulSets should be created with their expected replicas
	var sset1 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset1"}, &sset1))
	require.Equal(t, common.Int32(3), sset1.Spec.Replicas)
	var sset2 appsv1.StatefulSet
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, common.Int32(4), sset2.Spec.Replicas)
	// headless services should be created for both
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset1")}, &corev1.Service{}))
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: nodespec.HeadlessServiceName("sset2")}, &corev1.Service{}))
	// config should be created for both
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: name.ConfigSecret("sset1")}, &corev1.Secret{}))
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: name.ConfigSecret("sset2")}, &corev1.Secret{}))

	// upscale data nodes
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = common.Int32(10)
	err = HandleUpscaleAndSpecChanges(k8sClient, es, scheme.Scheme, expectedResources, actualStatefulSets)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, common.Int32(10), sset2.Spec.Replicas)

	// apply a spec change
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "b"}
	err = HandleUpscaleAndSpecChanges(k8sClient, es, scheme.Scheme, expectedResources, actualStatefulSets)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	require.Equal(t, "b", sset2.Spec.Template.Labels["a"])

	// apply a spec change and a downscale from 10 to 2
	actualStatefulSets = sset.StatefulSetList{sset1, sset2}
	expectedResources[1].StatefulSet.Spec.Replicas = common.Int32(2)
	expectedResources[1].StatefulSet.Spec.Template.Labels = map[string]string{"a": "c"}
	err = HandleUpscaleAndSpecChanges(k8sClient, es, scheme.Scheme, expectedResources, actualStatefulSets)
	require.NoError(t, err)
	require.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns", Name: "sset2"}, &sset2))
	// spec should be updated
	require.Equal(t, "c", sset2.Spec.Template.Labels["a"])
	// but StatefulSet should not be downscaled
	require.Equal(t, common.Int32(10), sset2.Spec.Replicas)
}
