// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package observer

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.elastic.co/apm/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	fixtures "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client/test_fixtures"
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
				cluster("first"):  {},
				cluster("second"): {},
			},
			want: []types.NamespacedName{cluster("first"), cluster("second")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(0, nil)
			m.observers = tt.observers
			require.ElementsMatch(t, tt.want, m.List())
		})
	}
}

func cluster(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "ns", Name: name}
}

func TestManager_Observe(t *testing.T) {
	fakeClient := fakeEsClient200(client.BasicAuth{})
	fakeClientWithDifferentUser := fakeEsClient200(client.BasicAuth{Name: "name", Password: "another-one"})
	defaultSettings := Settings{
		ObservationInterval: defaultObservationTimeout,
	}

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
			initiallyObserved:      map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, defaultSettings, nil)},
			clusterToObserve:       cluster("cluster2"),
			clusterToObserveClient: fakeClient,
			expectedObservers:      []types.NamespacedName{cluster("cluster"), cluster("cluster2")},
		},
		{
			name:                   "Observe twice the same cluster (idempotent)",
			initiallyObserved:      map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, defaultSettings, nil)},
			clusterToObserve:       cluster("cluster"),
			clusterToObserveClient: fakeClient,
			expectedObservers:      []types.NamespacedName{cluster("cluster")},
			expectNewObserver:      false,
		},
		{
			name:              "Observe twice the same cluster with a different client",
			initiallyObserved: map[types.NamespacedName]*Observer{cluster("cluster"): NewObserver(cluster("cluster"), fakeClient, defaultSettings, nil)},
			clusterToObserve:  cluster("cluster"),
			// more client comparison tests in client_test.go
			clusterToObserveClient: fakeClientWithDifferentUser,
			expectedObservers:      []types.NamespacedName{cluster("cluster")},
			expectNewObserver:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(10*time.Second, nil)
			m.observers = tt.initiallyObserved
			var initialCreationTime time.Time
			if initial, exists := tt.initiallyObserved[tt.clusterToObserve]; exists {
				initialCreationTime = initial.creationTime
			}
			esClientProvider := func(existingClient client.Client) client.Client { return tt.clusterToObserveClient }
			observer := m.Observe(context.Background(), esObject(tt.clusterToObserve), esClientProvider, true)
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

func flappingEsClient() client.Client {
	var retErr bool
	return client.NewMockClientWithUser(version.MustParse("8.3.0"),
		client.BasicAuth{},
		func(req *http.Request) *http.Response {
			if retErr {
				retErr = false
				return &http.Response{
					StatusCode: 503,
					Header:     make(http.Header),
					Request:    req,
				}
			}
			retErr = true
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewBufferString(fixtures.HealthSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
}

func TestManager_ObserveSync(t *testing.T) {
	tests := []struct {
		name           string
		manager        *Manager
		expectedHealth []esv1.ElasticsearchHealth
	}{
		{
			name:    "Async observation disabled make sync requests every time",
			manager: NewManager(-1*time.Second, nil),
			expectedHealth: []esv1.ElasticsearchHealth{
				esv1.ElasticsearchGreenHealth,
				// the flapping client returns an error on the second request
				esv1.ElasticsearchUnknownHealth,
			},
		},
		{
			name:    "Async observation enabled, only the first request is synchronous",
			manager: NewManager(1*time.Hour, nil),
			expectedHealth: []esv1.ElasticsearchHealth{
				esv1.ElasticsearchGreenHealth,
				// the async observer returns the old observation while the observation interval has not expired
				esv1.ElasticsearchGreenHealth,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			esClient := flappingEsClient()
			name := cluster("es1")
			cluster := esObject(name)
			esClientProvider := func(existingClient client.Client) client.Client { return esClient }
			results := []esv1.ElasticsearchHealth{
				tt.manager.ObservedStateResolver(context.Background(), cluster, esClientProvider, true)(),
				tt.manager.ObservedStateResolver(context.Background(), cluster, esClientProvider, true)(),
			}
			require.Equal(t, tt.expectedHealth, results)
			tt.manager.StopObserving(name) // let's clean up the go-routines
		})
	}
}

func TestManager_StopObserving(t *testing.T) {
	esClient := fakeEsClient200(client.BasicAuth{})
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
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster"): {esClient: esClient, stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("another-cluster")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster")},
		},
		{
			name:                       "stop observing the single cluster",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster"): {esClient: esClient, stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster")},
			expectedAfterStopObserving: []types.NamespacedName{},
		},
		{
			name:                       "stop observing one cluster",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster1"): {esClient: esClient, stopChan: make(chan struct{})}, cluster("cluster2"): {esClient: esClient, stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster2")},
		},
		{
			name:                       "stop observing the same cluster twice",
			observed:                   map[types.NamespacedName]*Observer{cluster("cluster1"): {esClient: esClient, stopChan: make(chan struct{})}, cluster("cluster2"): {esClient: esClient, stopChan: make(chan struct{})}},
			stopObserving:              []types.NamespacedName{cluster("cluster1"), cluster("cluster1")},
			expectedAfterStopObserving: []types.NamespacedName{cluster("cluster2")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(10*time.Second, nil)
			m.observers = tt.observed
			for _, name := range tt.stopObserving {
				m.StopObserving(name)
			}
			require.ElementsMatch(t, tt.expectedAfterStopObserving, m.List())
		})
	}
}

func TestManager_AddObservationListener(_ *testing.T) {
	m := NewManager(1*time.Second, nil)
	ctx := context.Background()

	cluster1 := esObject(cluster("cluster1"))
	cluster1.ObjectMeta.Annotations = map[string]string{ObserverIntervalAnnotation: "0.000001s"}

	cluster2 := esObject(cluster("cluster2"))
	cluster2.ObjectMeta.Annotations = map[string]string{ObserverIntervalAnnotation: "0.000001s"}

	// add a listener that is only interested in cluster1
	eventsCluster1 := make(chan types.NamespacedName)
	m.AddObservationListener(func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth) {
		if cluster.Name == "cluster1" {
			eventsCluster1 <- cluster
		}
	})

	// add a 2nd listener that is only interested in cluster2
	eventsCluster2 := make(chan types.NamespacedName)
	m.AddObservationListener(func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth) {
		if cluster.Name == "cluster2" {
			eventsCluster2 <- cluster
		}
	})
	doneCh := make(chan struct{})
	go func() {
		// events should be propagated to both listeners
		<-eventsCluster1
		<-eventsCluster2
		<-eventsCluster1
		<-eventsCluster2
		close(doneCh)
	}()
	esClientProvider := func(existingClient client.Client) client.Client { return fakeEsClient200(client.BasicAuth{}) }
	// observe 2 clusters
	obs1 := m.Observe(ctx, cluster1, esClientProvider, true)
	defer obs1.Stop()
	obs2 := m.Observe(ctx, cluster2, esClientProvider, true)
	defer obs2.Stop()
	<-doneCh
}

func esObject(n types.NamespacedName) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: n.Namespace,
			Name:      n.Name,
		},
	}
}

func TestExtractSettings(t *testing.T) {
	testCases := []struct {
		name           string
		globalInterval time.Duration
		annotations    map[string]string
		want           Settings
	}{
		{
			name:           "no annotations",
			globalInterval: 1 * time.Minute,
			want:           Settings{ObservationInterval: 1 * time.Minute},
		},
		{
			name:           "with annotations",
			globalInterval: 1 * time.Second,
			annotations:    map[string]string{ObserverIntervalAnnotation: "42s"},
			want:           Settings{ObservationInterval: 42 * time.Second},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test", Annotations: tc.annotations}}
			m := NewManager(tc.globalInterval, nil)
			have := m.extractObserverSettings(context.Background(), es)
			require.Equal(t, tc.want, have)
		})
	}
}

func TestSettingsComparison(t *testing.T) {
	testCases := []struct {
		name string
		s1   Settings
		s2   Settings
		want bool
	}{
		{
			name: "same settings (nil tracer)",
			s1:   Settings{ObservationInterval: 1 * time.Second},
			s2:   Settings{ObservationInterval: 1 * time.Second},
			want: true,
		},
		{
			name: "same settings (non nil tracer)",
			s1:   Settings{ObservationInterval: 1 * time.Second, Tracer: apm.DefaultTracer()},
			s2:   Settings{ObservationInterval: 1 * time.Second, Tracer: apm.DefaultTracer()},
			want: true,
		},
		{
			name: "different durations",
			s1:   Settings{ObservationInterval: 1 * time.Second},
			s2:   Settings{ObservationInterval: 2 * time.Second},
			want: false,
		},
		{
			name: "different tracers",
			s1:   Settings{ObservationInterval: 1 * time.Second, Tracer: apm.DefaultTracer()},
			s2:   Settings{ObservationInterval: 1 * time.Second},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.s1 == tc.s2)
		})
	}
}
