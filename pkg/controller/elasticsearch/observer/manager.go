// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package observer

import (
	"context"
	"sync"
	"time"

	"go.elastic.co/apm/v2"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	// ObserverIntervalAnnotation is the name of the annotation used to set the observation interval for a cluster.
	ObserverIntervalAnnotation = "eck.k8s.elastic.co/es-observer-interval"
)

// Manager for a set of observers
type Manager struct {
	defaultInterval time.Duration
	observerLock    sync.RWMutex
	observers       map[types.NamespacedName]*Observer
	listenerLock    sync.RWMutex
	listeners       []OnObservation // invoked on each observation event
	tracer          *apm.Tracer
}

// NewManager returns a new manager
func NewManager(defaultInterval time.Duration, tracer *apm.Tracer) *Manager {
	return &Manager{
		defaultInterval: defaultInterval,
		observers:       make(map[types.NamespacedName]*Observer),
		tracer:          tracer,
	}
}

// ObservedStateResolver returns a function that returns the last known state of the given cluster,
// as expected by the main reconciliation driver
func (m *Manager) ObservedStateResolver(
	ctx context.Context,
	cluster esv1.Elasticsearch,
	esClientProvider func(client.Client) client.Client,
	isServiceReady bool,
) func() esv1.ElasticsearchHealth {
	observer := m.Observe(ctx, cluster, esClientProvider, isServiceReady)
	return func() esv1.ElasticsearchHealth {
		return observer.LastHealth()
	}
}

func (m *Manager) getObserver(key types.NamespacedName) (*Observer, bool) {
	m.observerLock.RLock()
	defer m.observerLock.RUnlock()

	observer, ok := m.observers[key]
	return observer, ok
}

// Observe gets or create a cluster state observer for the given cluster
// In case something has changed in the given esClient (eg. different caCert), the observer is recreated accordingly
func (m *Manager) Observe(ctx context.Context, cluster esv1.Elasticsearch, esClientProvider func(client.Client) client.Client, isServiceReady bool) *Observer {
	defer tracing.Span(&ctx)()
	nsName := k8s.ExtractNamespacedName(&cluster)
	settings := m.extractObserverSettings(ctx, cluster)

	observer, exists := m.getObserver(nsName)

	var esClient client.Client
	if exists {
		esClient = esClientProvider(observer.esClient)
	} else {
		esClient = esClientProvider(nil)
	}

	switch {
	case !exists:
		// This Elasticsearch resource has not being observed yet, create the observer and maybe do a first observation.
		observer = m.createOrReplaceObserver(ctx, nsName, settings, esClient)
	case exists && (!observer.esClient.Equal(esClient) || observer.settings != settings):
		// This Elasticsearch resource is already being observed asynchronously, no need to do a first observation.
		observer = m.createOrReplaceObserver(ctx, nsName, settings, esClient)
	case exists && settings.ObservationInterval <= 0:
		// in case asynchronous observation has been disabled ensure at least one observation at reconciliation time.
		return m.getAndObserveSynchronously(ctx, nsName)
	default:
		// No change, return the existing observer.
		return observer
	}

	if !exists && isServiceReady {
		// there was no existing observer and Service is ready: let's try an initial synchronous observation
		observer.observe(ctx)
	}
	// start the new observer
	observer.Start()
	return observer
}

// extractObserverSettings extracts observer settings from the annotations on the Elasticsearch resource.
func (m *Manager) extractObserverSettings(ctx context.Context, cluster esv1.Elasticsearch) Settings {
	return Settings{
		ObservationInterval: annotation.ExtractTimeout(ctx, cluster.ObjectMeta, ObserverIntervalAnnotation, m.defaultInterval),
		Tracer:              m.tracer,
	}
}

// createOrReplaceObserver creates a new observer and adds it to the observers map, replacing existing observers if necessary.
// The new observer is not started, it is up to the caller to invoke observer.Start(ctx)
func (m *Manager) createOrReplaceObserver(ctx context.Context, cluster types.NamespacedName, settings Settings, esClient client.Client) *Observer {
	defer tracing.Span(&ctx)()
	m.observerLock.Lock()
	defer m.observerLock.Unlock()

	observer, exists := m.observers[cluster]
	if exists {
		log.Info("Replacing observer", "namespace", cluster.Namespace, "es_name", cluster.Name)
		observer.Stop()
		delete(m.observers, cluster)
	}
	observer = NewObserver(cluster, esClient, settings, m.notifyListeners)
	m.observers[cluster] = observer
	return observer
}

// getAndObserveSynchronously retrieves the currently configured observer and trigger a synchronous observation.
func (m *Manager) getAndObserveSynchronously(ctx context.Context, cluster types.NamespacedName) *Observer {
	defer tracing.Span(&ctx)()
	m.observerLock.RLock()
	defer m.observerLock.RUnlock()

	// invariant: this method must only be called when existence of observer is given
	observer := m.observers[cluster]
	// force a synchronous observation
	observer.observe(ctx)
	return observer
}

// List returns the names of clusters currently observed
func (m *Manager) List() []types.NamespacedName {
	m.observerLock.RLock()
	defer m.observerLock.RUnlock()

	names := make([]types.NamespacedName, len(m.observers))
	i := 0
	for name := range m.observers {
		names[i] = name
		i++
	}
	return names
}

// AddObservationListener adds the given listener to the list of listeners notified
// on every observation.
func (m *Manager) AddObservationListener(listener OnObservation) {
	m.listenerLock.Lock()
	defer m.listenerLock.Unlock()
	m.listeners = append(m.listeners, listener)
}

// notifyListeners notifies all listeners that an observation occurred.
func (m *Manager) notifyListeners(cluster types.NamespacedName, previousState, newState esv1.ElasticsearchHealth) {
	m.listenerLock.RLock()
	switch len(m.listeners) {
	case 0:
		m.listenerLock.RUnlock()
		return
	case 1:
		m.listeners[0](cluster, previousState, newState)
		m.listenerLock.RUnlock()
		return
	default:
		var wg sync.WaitGroup
		for _, l := range m.listeners {
			wg.Add(1)
			go func(f OnObservation) {
				f(cluster, previousState, newState)
				wg.Done()
			}(l)
		}
		m.listenerLock.RUnlock()
		wg.Wait()
	}
}

func (m *Manager) StopObserving(key types.NamespacedName) {
	log.Info("Stopping observer", "namespace", key.Namespace, "es_name", key.Name)
	m.observerLock.Lock()
	defer m.observerLock.Unlock()

	if observer, ok := m.observers[key]; ok {
		observer.Stop()
		delete(m.observers, key)
	}
}
