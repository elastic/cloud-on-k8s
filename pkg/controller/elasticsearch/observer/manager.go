// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"sync"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// ObserverIntervalAnnotation is the name of the annotation used to set the observation interval for a cluster.
	ObserverIntervalAnnotation = "eck.k8s.elastic.co/es-observer-interval"
)

// Manager for a set of observers
type Manager struct {
	observers map[types.NamespacedName]*Observer
	listeners []OnObservation // invoked on each observation event
	lock      sync.RWMutex
	tracer    *apm.Tracer
}

// NewManager returns a new manager
func NewManager(tracer *apm.Tracer) *Manager {
	return &Manager{
		observers: make(map[types.NamespacedName]*Observer),
		lock:      sync.RWMutex{},
		tracer:    tracer,
	}
}

// ObservedStateResolver returns the last known state of the given cluster,
// as expected by the main reconciliation driver
func (m *Manager) ObservedStateResolver(cluster esv1.Elasticsearch, esClient client.Client) State {
	return m.Observe(cluster, esClient).LastState()
}

// Observe gets or create a cluster state observer for the given cluster
// In case something has changed in the given esClient (eg. different caCert), the observer is recreated accordingly
func (m *Manager) Observe(cluster esv1.Elasticsearch, esClient client.Client) *Observer {
	nsName := k8s.ExtractNamespacedName(&cluster)
	settings := m.extractObserverSettings(cluster)

	m.lock.RLock()
	observer, exists := m.observers[nsName]
	m.lock.RUnlock()

	switch {
	case !exists:
		return m.createObserver(nsName, settings, esClient)
	case exists && (!observer.esClient.Equal(esClient) || observer.settings != settings):
		log.Info("Replacing observer HTTP client", "namespace", cluster.Namespace, "es_name", cluster.Name)
		m.StopObserving(nsName)
		return m.createObserver(nsName, settings, esClient)
	default:
		esClient.Close()
		return observer
	}
}

// extractObserverSettings extracts observer settings from the annotations on the Elasticsearch resource.
func (m *Manager) extractObserverSettings(cluster esv1.Elasticsearch) Settings {
	return Settings{
		ObservationInterval: annotation.ExtractTimeout(cluster.ObjectMeta, ObserverIntervalAnnotation, defaultObservationInterval),
		Tracer:              m.tracer,
	}
}

// createObserver creates a new observer according to the given arguments,
// and create/replace its entry in the observers map
func (m *Manager) createObserver(cluster types.NamespacedName, settings Settings, esClient client.Client) *Observer {
	observer := NewObserver(cluster, esClient, settings, m.notifyListeners)
	observer.Start()
	m.lock.Lock()
	m.observers[cluster] = observer
	m.lock.Unlock()
	return observer
}

// StopObserving stops and deletes the observer for the given cluster
// aimed to be called when an Elasticsearch resource is deleted.
func (m *Manager) StopObserving(cluster types.NamespacedName) {
	m.lock.RLock()
	observer, exists := m.observers[cluster]
	m.lock.RUnlock()
	if !exists {
		return
	}
	observer.Stop()
	m.lock.Lock()
	delete(m.observers, cluster)
	m.lock.Unlock()
}

// List returns the names of clusters currently observed
func (m *Manager) List() []types.NamespacedName {
	m.lock.RLock()
	defer m.lock.RUnlock()
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
	m.lock.Lock()
	defer m.lock.Unlock()
	m.listeners = append(m.listeners, listener)
}

// notifyListeners notifies all listeners that an observation occurred.
func (m *Manager) notifyListeners(cluster types.NamespacedName, previousState State, newState State) {
	wg := sync.WaitGroup{}
	m.lock.Lock()
	wg.Add(len(m.listeners))
	// run all listeners in parallel
	for _, l := range m.listeners {
		go func(f OnObservation) {
			defer wg.Done()
			f(cluster, previousState, newState)
		}(l)
	}
	// release the lock asap
	m.lock.Unlock()
	// wait for all listeners to be done
	wg.Wait()
}
