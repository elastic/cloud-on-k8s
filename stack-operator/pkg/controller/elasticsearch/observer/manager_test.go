package observer

import (
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
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
				clusterName("first"):  &Observer{},
				clusterName("second"): &Observer{},
			},
			want: []types.NamespacedName{clusterName("first"), clusterName("second")},
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

func clusterName(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "ns", Name: name}
}

func TestManager_Observe(t *testing.T) {
	fakeClient := fakeEsClient200()
	fakeClientWithDifferentUser := fakeEsClient200()
	fakeClientWithDifferentUser.User = client.User{Name: "name", Password: "another-one"}
	tests := []struct {
		name                   string
		initiallyObserved      map[types.NamespacedName]*Observer
		clusterToObserve       types.NamespacedName
		clusterToObserveClient *client.Client
		expectedObservers      []types.NamespacedName
		expectNewObserver      bool
	}{
		{
			name:                   "Observe a first cluster",
			initiallyObserved:      map[types.NamespacedName]*Observer{},
			clusterToObserve:       clusterName("cluster"),
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []types.NamespacedName{clusterName("cluster")},
		},
		{
			name:                   "Observe a second cluster",
			initiallyObserved:      map[types.NamespacedName]*Observer{clusterName("cluster"): NewObserver(clusterName("cluster"), &fakeClient, DefaultSettings)},
			clusterToObserve:       clusterName("cluster2"),
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []types.NamespacedName{clusterName("cluster"), clusterName("cluster2")},
		},
		{
			name:                   "Observe twice the same cluster (idempotent)",
			initiallyObserved:      map[types.NamespacedName]*Observer{clusterName("cluster"): NewObserver(clusterName("cluster"), &fakeClient, DefaultSettings)},
			clusterToObserve:       clusterName("cluster"),
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []types.NamespacedName{clusterName("cluster")},
			expectNewObserver:      false,
		},
		{
			name:              "Observe twice the same cluster with a different client",
			initiallyObserved: map[types.NamespacedName]*Observer{clusterName("cluster"): NewObserver(clusterName("cluster"), &fakeClient, DefaultSettings)},
			clusterToObserve:  clusterName("cluster"),
			// more client comparison tests in client_test.go
			clusterToObserveClient: &fakeClientWithDifferentUser,
			expectedObservers:      []types.NamespacedName{clusterName("cluster")},
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
			require.Equal(t, tt.clusterToObserve, observer.clusterName)
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
			stopObserving:              []types.NamespacedName{clusterName("cluster")},
			expectedAfterStopObserving: []types.NamespacedName{},
		},
		{
			name:                       "stop observing a non-existing cluster from 1 observer",
			observed:                   map[types.NamespacedName]*Observer{clusterName("cluster"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{clusterName("another-cluster")},
			expectedAfterStopObserving: []types.NamespacedName{clusterName("cluster")},
		},
		{
			name:                       "stop observing the single cluster",
			observed:                   map[types.NamespacedName]*Observer{clusterName("cluster"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{clusterName("cluster")},
			expectedAfterStopObserving: []types.NamespacedName{},
		},
		{
			name:                       "stop observing one cluster",
			observed:                   map[types.NamespacedName]*Observer{clusterName("cluster1"): &Observer{stopChan: make(chan struct{})}, clusterName("cluster2"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{clusterName("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{clusterName("cluster2")},
		},
		{
			name:                       "stop observing the same cluster twice",
			observed:                   map[types.NamespacedName]*Observer{clusterName("cluster1"): &Observer{stopChan: make(chan struct{})}, clusterName("cluster2"): &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{clusterName("cluster1"), clusterName("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{clusterName("cluster2")},
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
