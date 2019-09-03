// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/require"
)

func Test_defaultDriver_expectationsMet(t *testing.T) {
	d := &defaultDriver{DefaultDriverParameters{
		Expectations: expectations.NewExpectations(),
		Client:       k8s.WrapClient(fake.NewFakeClient()),
	}}

	// no expectations set
	met, err := d.expectationsMet(sset.StatefulSetList{})
	require.NoError(t, err)
	require.True(t, met)

	// a sset generation is expected
	statefulSet := sset.TestSset{Name: "sset"}.Build()
	statefulSet.Generation = 123
	d.Expectations.ExpectGeneration(statefulSet.ObjectMeta)
	// but not met yet
	statefulSet.Generation = 122
	met, err = d.expectationsMet(sset.StatefulSetList{statefulSet})
	require.NoError(t, err)
	require.False(t, met)
	// met now
	statefulSet.Generation = 123
	met, err = d.expectationsMet(sset.StatefulSetList{statefulSet})
	require.NoError(t, err)
	require.True(t, met)

	// we expect some sset replicas to exist
	// but corresponding pod does not exist
	statefulSet.Spec.Replicas = common.Int32(1)
	// expectations should not be met: we miss a pod
	met, err = d.expectationsMet(sset.StatefulSetList{statefulSet})
	require.NoError(t, err)
	require.False(t, met)

	// add the missing pod
	pod := sset.TestPod{Name: "sset-0", StatefulSetName: statefulSet.Name}.Build()
	d.Client = k8s.WrapClient(fake.NewFakeClient(&pod))
	// expectations should be met
	met, err = d.expectationsMet(sset.StatefulSetList{statefulSet})
	require.NoError(t, err)
	require.True(t, met)
}
