// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"
	"sync"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"
)

var log = ulog.Log.WithName("observer")

// Settings for the Observer configuration
type Settings struct {
	ObservationInterval time.Duration
	Tracer              *apm.Tracer
}

// defaultObservationInterval is the default interval of observation.
// if the Elasticsearch cluster is unavailable, the actual interval would be observationInterval + requestTimeout.
const defaultObservationInterval = 10 * time.Second

// OnObservation is a function that gets executed when a new state is observed
type OnObservation func(cluster types.NamespacedName, previousState State, newState State)

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	cluster       types.NamespacedName
	esClient      client.Client
	settings      Settings
	creationTime  time.Time
	stopChan      chan struct{}
	stopOnce      sync.Once
	onObservation OnObservation
	lastState     State
	mutex         sync.RWMutex
}

// NewObserver creates and starts an Observer
func NewObserver(cluster types.NamespacedName, esClient client.Client, settings Settings, onObservation OnObservation) *Observer {
	observer := Observer{
		cluster:       cluster,
		esClient:      esClient,
		creationTime:  time.Now(),
		settings:      settings,
		stopChan:      make(chan struct{}),
		stopOnce:      sync.Once{},
		onObservation: onObservation,
		mutex:         sync.RWMutex{},
	}

	log.Info("Creating observer for cluster", "namespace", cluster.Namespace, "es_name", cluster.Name)
	return &observer
}

// Start the observer in a separate goroutine
func (o *Observer) Start() {
	go o.runPeriodically()
}

// Stop the observer loop
func (o *Observer) Stop() {
	o.stopOnce.Do(func() {
		close(o.stopChan)
		o.esClient.Close()
	})
}

// LastState returns the last observed state
func (o *Observer) LastState() State {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastState
}

// runPeriodically triggers a state retrieval every tick,
// until the given context is cancelled
func (o *Observer) runPeriodically() {
	o.retrieveState()

	ticker := time.NewTicker(o.settings.ObservationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.retrieveState()
		case <-o.stopChan:
			log.Info("Stopping observer for cluster", "namespace", o.cluster.Namespace, "es_name", o.cluster.Name)
			return
		}
	}
}

// retrieveState retrieves the current ES state, executes onObservation,
// and stores the new state
func (o *Observer) retrieveState() {
	ctx, cancelFunc := context.WithTimeout(context.Background(), o.settings.ObservationInterval)
	defer cancelFunc()

	log.V(1).Info("Retrieving cluster state", "es_name", o.cluster.Name, "namespace", o.cluster.Namespace)

	if o.settings.Tracer != nil {
		tx := o.settings.Tracer.StartTransaction(o.cluster.String(), "elasticsearch_observer")
		defer tx.End()
		ctx = apm.ContextWithTransaction(ctx, tx)
	}

	newState := RetrieveState(ctx, o.cluster, o.esClient)

	if o.onObservation != nil {
		o.onObservation(o.cluster, o.LastState(), newState)
	}

	o.mutex.Lock()
	o.lastState = newState
	o.mutex.Unlock()
}
