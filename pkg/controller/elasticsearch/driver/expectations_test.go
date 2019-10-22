// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func Test_defaultDriver_expectationSatisfied(t *testing.T) {
	client := k8s.WrappedFakeClient()
	es := v1beta1.Elasticsearch{
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
	satisfied, err := d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)

	// a sset generation is expected
	statefulSet := sset.TestSset{Namespace: es.Namespace, Name: "sset", ClusterName: es.Name}.Build()
	statefulSet.Generation = 123
	d.Expectations.ExpectGeneration(statefulSet)
	// but not satisfied yet
	statefulSet.Generation = 122
	require.NoError(t, client.Create(&statefulSet))
	satisfied, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.False(t, satisfied)
	// satisfied now
	statefulSet.Generation = 123
	require.NoError(t, client.Update(&statefulSet))
	satisfied, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)

	// we expect some sset replicas to exist
	// but corresponding pod does not exist yet
	statefulSet.Spec.Replicas = pointer.Int32(1)
	require.NoError(t, client.Update(&statefulSet))
	// expectations should not be satisfied: we miss a pod
	satisfied, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// add the missing pod
	pod := sset.TestPod{Namespace: es.Namespace, Name: "sset-0", StatefulSetName: statefulSet.Name}.Build()
	require.NoError(t, client.Create(&pod))
	// expectations should be satisfied
	satisfied, err = d.expectationsSatisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
}
