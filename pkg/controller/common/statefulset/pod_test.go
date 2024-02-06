// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package statefulset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// Test that we actually filter on the sset name and the namespace
func TestGetActualPodsForStatefulSet(t *testing.T) {
	objs := []client.Object{
		getPodSample("pod0", "ns0", "sset0", "clus0", "0"),
		getPodSample("pod1", "ns1", "sset0", "clus0", "0"),
		getPodSample("pod2", "ns0", "sset1", "clus1", "0"),
		getPodSample("pod3", "ns0", "sset1", "clus0", "0"),
	}
	c := k8s.NewFakeClient(objs...)
	sset0 := getSsetSample("sset0", "ns0", "clus0")
	pods, err := GetActualPodsForStatefulSet(c, k8s.ExtractNamespacedName(&sset0), label.StatefulSetNameLabelName)
	require.NoError(t, err)
	// only one pod is in the same stateful set and namespace
	assert.Equal(t, 1, len(pods))
}

func getPodSample(name, namespace, ssetName, clusterName, revision string) *corev1.Pod {
	return TestPod{
		Namespace:       namespace,
		Name:            name,
		ClusterName:     clusterName,
		StatefulSetName: ssetName,
		Revision:        revision,
	}.BuildPtr()
}

func getSsetSample(name, namespace, clusterName string) appsv1.StatefulSet {
	return TestSset{
		Name:        name,
		Namespace:   namespace,
		ClusterName: clusterName,
		Replicas:    3,
		Status: appsv1.StatefulSetStatus{
			CurrentRevision: "1",
			UpdateRevision:  "1",
		},
	}.Build()
}
