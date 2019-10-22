// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/uuid"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestExpectations_Satisfied(t *testing.T) {
	client := k8s.WrappedFakeClient()
	e := NewExpectations(client)

	// initially satisfied
	satisfied, err := e.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)

	// expect a Pod to be deleted
	pod := newPod("pod1", uuid.NewUUID())
	require.NoError(t, client.Create(&pod))
	e.ExpectDeletion(pod)

	// expectations should not be satisfied
	satisfied, err = e.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// expect a StatefulSet generation
	sset := newStatefulSet("sset1", uuid.NewUUID(), 1)
	require.NoError(t, client.Create(&sset))
	updatedSset := sset
	updatedSset.Generation = 2
	e.ExpectGeneration(updatedSset)

	// expectations should not be satisfied
	satisfied, err = e.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// observe the StatefulSet with the updated generation
	require.NoError(t, client.Update(&updatedSset))
	// expectations should not be satisfied (because of the deletions)
	satisfied, err = e.Satisfied()
	require.NoError(t, err)
	require.False(t, satisfied)

	// delete the Pod
	require.NoError(t, client.Delete(&pod))
	// expectations should be satisfied
	satisfied, err = e.Satisfied()
	require.NoError(t, err)
	require.True(t, satisfied)
}
