// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"crypto/x509"
	"sync"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"k8s.io/apimachinery/pkg/types"
)

// Manager for a set of observers
type Manager struct {
	k8sClient k8s.Client
	dialer    net.Dialer
	observers map[types.NamespacedName]*Observer
	listeners []OnObservation // invoked on each observation event
	lock      sync.RWMutex
	settings  Settings
}

// NewManager returns a new manager
func NewManager(dialer net.Dialer, k8sClient k8s.Client, settings Settings) *Manager {
	return &Manager{
		k8sClient: k8sClient,
		dialer:    dialer,
		observers: make(map[types.NamespacedName]*Observer),
		lock:      sync.RWMutex{},
		settings:  settings,
	}
}

// ObservedStateResolver returns the last known state of the given cluster,
// as expected by the main reconciliation driver
func (m *Manager) ObservedStateResolver(cluster types.NamespacedName, caCerts []*x509.Certificate, esClient client.Client) State {
	return m.Observe(cluster, caCerts, esClient).LastState()
}

// Observe gets or create a cluster state observer for the given cluster
// In case something has changed in the given esClient (eg. different caCert), the observer is recreated accordingly
func (m *Manager) Observe(cluster types.NamespacedName, caCerts []*x509.Certificate, esClient client.Client) *Observer {
	m.lock.RLock()
	observer, exists := m.observers[cluster]
	m.lock.RUnlock()

	switch {
	case !exists:
		return m.createObserver(cluster, caCerts, esClient)
	case exists && !observer.esClient.Equal(esClient):
		log.Info("Replacing observer HTTP client", "cluster", cluster)
		m.StopObserving(cluster)
		return m.createObserver(cluster, caCerts, esClient)
	default:
		return observer
	}
}

// createObserver creates a new observer according to the given arguments,
// and create/replace its entry in the observers map
func (m *Manager) createObserver(cluster types.NamespacedName, caCerts []*x509.Certificate, esClient client.Client) *Observer {
	observer := NewObserver(m.k8sClient, m.dialer, caCerts, cluster, esClient, m.settings, m.notifyListeners)
	observer.Start()
	m.lock.Lock()
	m.observers[cluster] = observer
	m.lock.Unlock()
	return observer
}

// StopObserving stops and deletes the observer for the given cluster
// aimed to be called automatically by a finalizer
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
