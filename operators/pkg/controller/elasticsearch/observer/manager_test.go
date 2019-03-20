// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestManager_List(t *testing.T) {
	tests := []struct {
		name      string
		observers map[types.NamespacedName]*Observer
		want      []types.NamespacedName
	}{
		{
			name:      "Empty list",
			observers: map[types.NamespacedName]*Observer{},
			want:      []types.NamespacedName{},
		},
		{
			name: "Non-empty list",
			observers: map[types.NamespacedName]*Observer{
				cluster("first"):  &Observer{},
				cluster("second"): &Observer{},
			},
			want: []types.NamespacedName{cluster("first"), cluster("second")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(DefaultSettings)
			m.observers = tt.observers
			require.ElementsMatch(t, tt.want, m.List())
		})
	}
}

func cluster(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "ns", Name: name}
}

func TestManager_Observe(t *testing.T) {
	fakeClient := fakeEsClient200(client.UserAuth{})
	fakeClientWithDifferentUser := fakeEsClient200(client.UserAuth{Name: "name", Password: "another-one"})
	tests := []struct {
		name                   string
		initiallyObserved      map[types.NamespacedName]*Observer
		clusterToObserve       types.NamespacedName
		clusterToObserveClient client.Client
		expectedObservers      []types.NamespacedName
		expectNewObserver      bool
	}{
		{
			name:                   "Observe a first cluster",
			initiallyObserved:      map[types.NamespacedName]*Observer{},
			clusterToObserve:       cluster("cluster"),
			clusterToObserveClient: fakeClient,
			expectedObservers:      []types.NamespacedName{cluster("cluster")},
		},
		{
			name:                   "Observe a second cluster",
			initiallyObserved:      map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, DefaultSettings)},
			clusterToObserve:       cluster("cluster2"),
			clusterToObserveClient: fakeClient,
			expectedObservers:      []types.NamespacedName{cluster("cluster"), cluster("cluster2")},
		},
		{
			name:                   "Observe twice the same cluster (idempotent)",
			initiallyObserved:      map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, DefaultSettings)},
			clusterToObserve:       cluster("cluster"),
			clusterToObserveClient: fakeClient,
			expectedObservers:      []types.NamespacedName{cluster("cluster")},
			expectNewObserver:      false,
		},
		{
			name:              "Observe twice the same cluster with a different client",
			initiallyObserved: map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, DefaultSettings)},
			clusterToObserve:  cluster("cluster"),
			// more client comparison tests in client_test.go
			clusterToObserveClient: fakeClientWithDifferentUser,
			expectedObservers:      []types.NamespacedName{cluster("cluster")},
			expectNewObserver:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(DefaultSettings)
			m.observers = tt.initiallyObserved
			var initialCreationTime time.Time
			if initial, exists := tt.initiallyObserved[tt.clusterToObserve]; exists {
				initialCreationTime = initial.creationTime
			}
			observer := m.Observe(tt.clusterToObserve, tt.clusterToObserveClient)
			// returned observer should be the correct one
			require.Equal(t, tt.clusterToObserve, observer.cluster)
			// list of observers should have been updated
			require.ElementsMatch(t, tt.expectedObservers, m.List())
			if !initialCreationTime.IsZero() {
				// observer may have been replaced
				require.Equal(t, tt.expectNewObserver, !initialCreationTime.Equal(observer.creationTime))
			}
			observer.Stop()
		})
	}
}

func TestManager_StopObserving(t *testing.T) {
	tests := []struct {
		name                       string
		observed                   map[types.NamespacedName]*Observer
		stopObserving              []types.NamespacedName
		expectedAfterStopObserving []types.NamespacedName
	}{
		{
			name:                       "stop observing a non-existing cluster from no observers",
			observed:                   map[types.NamespacedName]*Observer{},
			stopObserving:              []types.NamespacedName{cluster("cluster")},
			expectedAfterStopObserving: []types.NamespacedName{},
		},
		{
			name:                       "stop observing a non-existing cluster from 1 observer",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("another-cluster")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster")},
		},
		{
			name:                       "stop observing the single cluster",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster")},
			expectedAfterStopObserving: []types.NamespacedName{},
		},
		{
			name:                       "stop observing one cluster",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster1"): &Observer{stopChan: make(chan struct{})}, cluster("cluster2"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster2")},
		},
		{
			name:                       "stop observing the same cluster twice",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster1"): &Observer{stopChan: make(chan struct{})}, cluster("cluster2"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster1"), cluster("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster2")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(DefaultSettings)
			m.observers = tt.observed
			for _, name := range tt.stopObserving {
				m.StopObserving(name)
			}
			require.ElementsMatch(t, tt.expectedAfterStopObserving, m.List())
		})
	}
}
