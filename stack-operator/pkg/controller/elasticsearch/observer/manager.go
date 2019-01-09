package observer

import (
	"sync"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
)

// Manager for a set of observers
type Manager struct {
	observers map[string]*Observer
	lock      sync.RWMutex
	settings  Settings
}

// NewManager returns a new manager
func NewManager(settings Settings) *Manager {
	return &Manager{
		observers: make(map[string]*Observer),
		lock:      sync.RWMutex{},
		settings:  settings,
	}
}

// ObservedStateResolver returns the last known state of the given cluster,
// as expected by the main reconciliation driver
func (m *Manager) ObservedStateResolver(esCluster elasticsearchv1alpha1.ElasticsearchCluster, esClient *client.Client) State {
	return m.Observe(esCluster, esClient).LastState()
}

// Observe gets or create a cluster state observer for the given cluster
// In case something has changed in the given esClient (eg. different caCert), the observer is recreated accordingly
func (m *Manager) Observe(esCluster elasticsearchv1alpha1.ElasticsearchCluster, esClient *client.Client) *Observer {
	m.lock.RLock()
	observer, exists := m.observers[esCluster.Name]
	m.lock.RUnlock()

	switch {
	case !exists:
		return m.createObserver(esCluster, esClient)
	case exists && !observer.esClient.Equal(esClient):
		log.Info("Replacing observer HTTP client", "cluster", esCluster.Name)
		m.StopObserving(esCluster)
		return m.createObserver(esCluster, esClient)
	default:
		return observer
	}
}

// createObserver creates a new observer according to the given arguments,
// and create/replace its entry in the observers map
func (m *Manager) createObserver(esCluster elasticsearchv1alpha1.ElasticsearchCluster, esClient *client.Client) *Observer {
	observer := NewObserver(esCluster.Name, esClient, m.settings)
	m.lock.Lock()
	m.observers[esCluster.Name] = observer
	m.lock.Unlock()
	return observer
}

// StopObserving stops and deletes the observer for the given cluster
// aimed to be called automatically by a finalizer
func (m *Manager) StopObserving(esCluster elasticsearchv1alpha1.ElasticsearchCluster) {
	m.lock.RLock()
	observer, exists := m.observers[esCluster.Name]
	m.lock.RUnlock()
	if !exists {
		return
	}
	observer.Stop()
	m.lock.Lock()
	delete(m.observers, esCluster.Name)
	m.lock.Unlock()
}

// List returns the names of clusters currently observed
func (m *Manager) List() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	list := []string{}
	for name := range m.observers {
		list = append(list, name)
	}
	return list
}
