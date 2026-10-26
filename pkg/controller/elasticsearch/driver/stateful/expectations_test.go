// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/expectations"
	sset "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/statefulset"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func Test_Driver_expectationSatisfied(t *testing.T) {
	client := k8s.NewFakeClient()
	es := esv1.Elasticsearch{
		Namespace: "ns",
		Name:      "cluster",
	}
	d := &Driver{
		Expectations: expectations.NewExpectations(client, &appsv1.StatefulSet{}),
		Client:       client,
		ES:           es}
	ctx := context.Background()

	// no expectations set
	satisfied, reason, err := d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

	// a sset generation is expected
	statefulSet := sset.TestSset{Namespace: es.Namespace, Name: "sset", ClusterName: es.Name}.Build()
	statefulSet.Generation = 123
	d.Expectations.ExpectGeneration(&statefulSet)
	// but not satisfied yet
	statefulSet.Generation = 122
	require.NoError(t, client.Create(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)
	// satisfied now, but not from the StatefulSet controller point of view (status.observedGeneration)
	statefulSet.Generation = 123
	require.NoError(t, client.Update(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)
	// satisfied now, with matching status.observedGeneration
	statefulSet.Status.ObservedGeneration = 123
	require.NoError(t, client.Status().Update(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

	// we expect some sset replicas to exist
	// but corresponding pod does not exist yet
	statefulSet.Spec.Replicas = new(int32(1))
	require.NoError(t, client.Update(context.Background(), &statefulSet))
	// expectations should not be satisfied: we miss a pod
	satisfied, reason, err = d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)

	// add the missing pod
	pod := sset.TestPod{Namespace: es.Namespace, Name: "sset-0", StatefulSetName: statefulSet.Name}.Build()
	require.NoError(t, client.Create(context.Background(), &pod))
	// expectations should be satisfied
	satisfied, reason, err = d.expectationsSatisfied(ctx)
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)
}

func Test_Driver_nodeSpecsExpectationsSatisfied(t *testing.T) {
	const namespace = "ns"
	const esName = "cluster"
	statefulSetName := esv1.StatefulSet(esName, "data")
	pod := func(ordinal int32) crclient.Object {
		return sset.TestPod{Namespace: namespace, Name: sset.PodName(statefulSetName, ordinal), StatefulSetName: statefulSetName, ClusterName: esName}.BuildPtr()
	}
	tests := []struct {
		name string
		// nodeSets are the NodeSets of the spec
		nodeSets []esv1.NodeSet
		// replicas and createdReplicas are the replicas of the StatefulSet, and the ones its controller reports created
		replicas, createdReplicas int32
		pods                      []crclient.Object
		// wantStrict is whether expectationsSatisfied, which the other steps keep relying on, is satisfied
		want, wantStrict bool
	}{
		{
			name:            "all the Pods exist",
			nodeSets:        []esv1.NodeSet{{Name: "data", Count: 2}},
			replicas:        2,
			createdReplicas: 2,
			pods:            []crclient.Object{pod(0), pod(1)},
			want:            true,
			wantStrict:      true,
		},
		{
			name:            "a Pod of a removed NodeSet could not be created",
			replicas:        1,
			createdReplicas: 0,
			want:            true,
		},
		{
			name:            "a Pod that a downscale removes could not be created",
			nodeSets:        []esv1.NodeSet{{Name: "data", Count: 2}},
			replicas:        3,
			createdReplicas: 2,
			pods:            []crclient.Object{pod(0), pod(1)},
			want:            true,
		},
		{
			name:            "a Pod that the spec keeps could not be created",
			nodeSets:        []esv1.NodeSet{{Name: "data", Count: 2}},
			replicas:        3,
			createdReplicas: 2,
			pods:            []crclient.Object{pod(0), pod(2)},
		},
		{
			name:            "a Pod of a removed NodeSet is created but not in the cache yet",
			replicas:        1,
			createdReplicas: 1,
		},
		{
			name:            "a Pod that a downscale removes could not be created while another one is still to be deleted",
			nodeSets:        []esv1.NodeSet{{Name: "data", Count: 1}},
			replicas:        2,
			createdReplicas: 2,
			pods:            []crclient.Object{pod(0), pod(2)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statefulSet := sset.TestSset{
				Namespace:   namespace,
				Name:        statefulSetName,
				ClusterName: esName,
				Replicas:    tt.replicas,
				Status:      appsv1.StatefulSetStatus{Replicas: tt.createdReplicas},
			}.BuildPtr()
			k8sClient := k8s.NewFakeClient(append([]crclient.Object{statefulSet}, tt.pods...)...)
			d := &Driver{
				Expectations: expectations.NewExpectations(k8sClient, &appsv1.StatefulSet{}),
				Client:       k8sClient,
				ES: esv1.Elasticsearch{
					Namespace: namespace,
					Name:      esName,
					Spec:      esv1.ElasticsearchSpec{NodeSets: tt.nodeSets},
				},
			}

			satisfied, reason, err := d.nodeSpecsExpectationsSatisfied(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.want, satisfied, reason)
			satisfied, reason, err = d.expectationsSatisfied(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantStrict, satisfied, reason)
		})
	}
}
