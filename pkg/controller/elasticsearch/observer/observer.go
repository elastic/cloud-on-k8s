// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package observer

import (
	"context"
	"sync"
	"time"

	"go.elastic.co/apm"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
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
type OnObservation func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth)

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
	lastHealth    esv1.ElasticsearchHealth
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
		lastHealth:    esv1.ElasticsearchUnknownHealth, // We don't know the health of the cluster until a first query succeeds
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

// LastHealth returns the last observed state
func (o *Observer) LastHealth() esv1.ElasticsearchHealth {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastHealth
}

// runPeriodically triggers a state retrieval every tick,
// until the given context is cancelled
func (o *Observer) runPeriodically() {
	o.observe()

	ticker := time.NewTicker(o.settings.ObservationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.observe()
		case <-o.stopChan:
			log.Info("Stopping observer for cluster", "namespace", o.cluster.Namespace, "es_name", o.cluster.Name)
			return
		}
	}
}

// observe retrieves the current ES state, executes onObservation,
// and stores the new state
func (o *Observer) observe() {
	ctx, cancelFunc := context.WithTimeout(context.Background(), o.settings.ObservationInterval)
	defer cancelFunc()

	log.V(1).Info("Retrieving cluster state", "es_name", o.cluster.Name, "namespace", o.cluster.Namespace)

	if o.settings.Tracer != nil {
		tx := o.settings.Tracer.StartTransaction(o.cluster.String(), "elasticsearch_observer")
		defer tx.End()
		ctx = apm.ContextWithTransaction(ctx, tx)
	}

	newHealth := retrieveHealth(ctx, o.cluster, o.esClient)
	if o.onObservation != nil {
		o.onObservation(o.cluster, o.LastHealth(), newHealth)
	}

	o.mutex.Lock()
	o.lastHealth = newHealth
	o.mutex.Unlock()
}

// retrieveHealth returns the current Elasticsearch cluster health
func retrieveHealth(ctx context.Context, cluster types.NamespacedName, esClient esclient.Client) esv1.ElasticsearchHealth {
	health, err := esClient.GetClusterHealth(ctx)
	if err != nil {
		log.V(1).Info(
			"Unable to retrieve cluster health",
			"error", err,
			"namespace", cluster.Namespace,
			"es_name", cluster.Name,
		)
		return esv1.ElasticsearchUnknownHealth
	}
	return health.Status
}
