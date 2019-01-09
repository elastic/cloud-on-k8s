package observer

import (
	"testing"
	"time"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManager_List(t *testing.T) {
	tests := []struct {
		name      string
		observers map[string]*Observer
		want      []string
	}{
		{
			name:      "Empty list",
			observers: map[string]*Observer{},
			want:      []string{},
		},
		{
			name: "Non-empty list",
			observers: map[string]*Observer{
				"first":  &Observer{},
				"second": &Observer{},
			},
			want: []string{"first", "second"},
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

func TestManager_Observe(t *testing.T) {
	fakeClient := fakeEsClient200()
	fakeClientWithDifferentUser := fakeEsClient200()
	fakeClientWithDifferentUser.User = client.User{Name: "name", Password: "another-one"}
	tests := []struct {
		name                   string
		initiallyObserved      map[string]*Observer
		clusterToObserve       string
		clusterToObserveClient *client.Client
		expectedObservers      []string
		expectNewObserver      bool
	}{
		{
			name:                   "Observe a first cluster",
			initiallyObserved:      map[string]*Observer{},
			clusterToObserve:       "cluster",
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []string{"cluster"},
		},
		{
			name:                   "Observe a second cluster",
			initiallyObserved:      map[string]*Observer{"cluster": NewObserver("cluster", &fakeClient, DefaultSettings)},
			clusterToObserve:       "cluster2",
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []string{"cluster", "cluster2"},
		},
		{
			name:                   "Observe twice the same cluster (idempotent)",
			initiallyObserved:      map[string]*Observer{"cluster": NewObserver("cluster", &fakeClient, DefaultSettings)},
			clusterToObserve:       "cluster",
			clusterToObserveClient: &fakeClient,
			expectedObservers:      []string{"cluster"},
			expectNewObserver:      false,
		},
		{
			name:              "Observe twice the same cluster with a different client",
			initiallyObserved: map[string]*Observer{"cluster": NewObserver("cluster", &fakeClient, DefaultSettings)},
			clusterToObserve:  "cluster",
			// more client comparison tests in client_test.go
			clusterToObserveClient: &fakeClientWithDifferentUser,
			expectedObservers:      []string{"cluster"},
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
			cluster := elasticsearchv1alpha1.ElasticsearchCluster{
				ObjectMeta: v1.ObjectMeta{
					Name: tt.clusterToObserve,
				},
			}
			observer := m.Observe(cluster, tt.clusterToObserveClient)
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
		observed                   map[string]*Observer
		stopObserving              []string
		expectedAfterStopObserving []string
	}{
		{
			name:                       "stop observing a non-existing cluster from no observers",
			observed:                   map[string]*Observer{},
			stopObserving:              []string{"cluster"},
			expectedAfterStopObserving: []string{},
		},
		{
			name:                       "stop observing a non-existing cluster from 1 observer",
			observed:                   map[string]*Observer{"cluster": &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []string{"another-cluster"},
			expectedAfterStopObserving: []string{"cluster"},
		},
		{
			name:                       "stop observing the single cluster",
			observed:                   map[string]*Observer{"cluster": &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []string{"cluster"},
			expectedAfterStopObserving: []string{},
		},
		{
			name:                       "stop observing one cluster",
			observed:                   map[string]*Observer{"cluster1": &Observer{stopChan: make(chan struct{})}, "cluster2": &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []string{"cluster1"},
			expectedAfterStopObserving: []string{"cluster2"},
		},
		{
			name:                       "stop observing the same cluster twice",
			observed:                   map[string]*Observer{"cluster1": &Observer{stopChan: make(chan struct{})}, "cluster2": &Observer{stopChan: make(chan struct{})}},
			stopObserving:              []string{"cluster1", "cluster1"},
			expectedAfterStopObserving: []string{"cluster2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(DefaultSettings)
			m.observers = tt.observed
			for _, name := range tt.stopObserving {
				cluster := elasticsearchv1alpha1.ElasticsearchCluster{
					ObjectMeta: v1.ObjectMeta{
						Name: name,
					},
				}
				m.StopObserving(cluster)
			}
			require.ElementsMatch(t, tt.expectedAfterStopObserving, m.List())
		})
	}
}
