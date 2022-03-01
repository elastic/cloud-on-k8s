// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package driver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func Test_defaultDriver_expectationSatisfied(t *testing.T) {
	client := k8s.NewFakeClient()
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "cluster",
		},
	}
	d := &defaultDriver{DefaultDriverParameters{
		Expectations: expectations.NewExpectations(client),
		Client:       client,
		ES:           es,
	}}

	// no expectations set
	satisfied, reason, err := d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

	// a sset generation is expected
	statefulSet := sset.TestSset{Namespace: es.Namespace, Name: "sset", ClusterName: es.Name}.Build()
	statefulSet.Generation = 123
	d.Expectations.ExpectGeneration(statefulSet)
	// but not satisfied yet
	statefulSet.Generation = 122
	require.NoError(t, client.Create(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)
	// satisfied now, but not from the StatefulSet controller point of view (status.observedGeneration)
	statefulSet.Generation = 123
	require.NoError(t, client.Update(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)
	// satisfied now, with matching status.observedGeneration
	statefulSet.Status.ObservedGeneration = 123
	require.NoError(t, client.Update(context.Background(), &statefulSet))
	satisfied, reason, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)

	// we expect some sset replicas to exist
	// but corresponding pod does not exist yet
	statefulSet.Spec.Replicas = pointer.Int32(1)
	require.NoError(t, client.Update(context.Background(), &statefulSet))
	// expectations should not be satisfied: we miss a pod
	satisfied, reason, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	require.NotEqual(t, "", reason)

	// add the missing pod
	pod := sset.TestPod{Namespace: es.Namespace, Name: "sset-0", StatefulSetName: statefulSet.Name}.Build()
	require.NoError(t, client.Create(context.Background(), &pod))
	// expectations should be satisfied
	satisfied, reason, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
	require.Equal(t, "", reason)
}
