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
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const name = "elasticsearch-observer"

var log = ulog.Log.WithName(name)

// Settings for the Observer configuration
type Settings struct {
	ObservationInterval time.Duration
	Tracer              *apm.Tracer
}

// defaultObservationTimeout is the default timeout for an observation. The observer uses the observation interval as a timeout.
// The default applies if the observation interval is not positive to allow at least one successful observation.
const defaultObservationTimeout = 10 * time.Second

// OnObservation is a function that gets executed when a new state is observed
type OnObservation func(cluster types.NamespacedName, previousState, newState esv1.ElasticsearchState)

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	cluster       types.NamespacedName
	esClient      esclient.Client
	settings      Settings
	creationTime  time.Time
	stopChan      chan struct{}
	stopOnce      sync.Once
	onObservation OnObservation
	lastState     esv1.ElasticsearchState
	mutex         sync.RWMutex
}

// NewObserver creates and starts an Observer
func NewObserver(cluster types.NamespacedName, esClient esclient.Client, settings Settings, onObservation OnObservation) *Observer {
	observer := Observer{
		cluster:       cluster,
		esClient:      esClient,
		creationTime:  time.Now(),
		settings:      settings,
		stopChan:      make(chan struct{}),
		stopOnce:      sync.Once{},
		onObservation: onObservation,
		lastState:     esv1.ElasticsearchState{ElasticsearchHealth: esv1.ElasticsearchUnknownHealth, NodesStats: esclient.NodesStats{}}, // We don't know the health of the cluster until a first query succeeds
		mutex:         sync.RWMutex{},
	}

	log.Info("Creating observer for cluster", "namespace", cluster.Namespace, "es_name", cluster.Name)
	return &observer
}

// Start starts the Observer in a separate goroutine after a first synchronous observation.
// The first observation is synchronous to allow to retrieve the cluster state immediately after the start.
// Then, observations are performed periodically in a separate goroutine until the observer stop channel is closed.
func (o *Observer) Start() {
	if o.settings.ObservationInterval <= 0 {
		return // asynchronous observations are effectively disabled
	}
	// periodic asynchronous observations
	go func() {
		ticker := time.NewTicker(o.settings.ObservationInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				o.observe(context.Background())
			case <-o.stopChan:
				log.Info("Stopping observer for cluster", "namespace", o.cluster.Namespace, "es_name", o.cluster.Name)
				return
			}
		}
	}()
}

// Stop the observer loop
func (o *Observer) Stop() {
	o.stopOnce.Do(func() {
		close(o.stopChan)
		o.esClient.Close()
	})
}

// LastState returns the last observed state
func (o *Observer) LastState() esv1.ElasticsearchState {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastState
}

// observe retrieves the current ES state, executes onObservation,
// and stores the new state
func (o *Observer) observe(ctx context.Context) {
	defer tracing.Span(&ctx)()
	ctx, cancelFunc := context.WithTimeout(ctx, nonNegativeTimeout(o.settings.ObservationInterval))
	defer cancelFunc()

	if o.settings.Tracer != nil && apm.TransactionFromContext(ctx) == nil {
		tx := o.settings.Tracer.StartTransaction(name, string(tracing.PeriodicTxType))
		defer tx.End()
		ctx = apm.ContextWithTransaction(ctx, tx)
	}
	// initialise logger after tracing has been started
	ctx = ulog.InitInContext(ctx, name)
	ulog.FromContext(ctx).V(1).Info("Retrieving cluster health and nodes stats", "es_name", o.cluster.Name, "namespace", o.cluster.Namespace)

	newHealth := retrieveHealth(ctx, o.cluster, o.esClient)
	newNodesStats := retrieveNodeStats(ctx, o.cluster, o.esClient)
	newState := esv1.ElasticsearchState{ElasticsearchHealth: newHealth, NodesStats: newNodesStats}
	if o.onObservation != nil {
		o.onObservation(o.cluster, o.LastState(), newState)
	}
	o.updateClusterObserver(newState)
}

func (o *Observer) updateClusterObserver(newState esv1.ElasticsearchState) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.lastState = newState
}

func nonNegativeTimeout(observationInterval time.Duration) time.Duration {
	// if the observation interval is not positive async observations are disabled
	if observationInterval <= 0 {
		// use a default positive timeout to allow one synchronous observation
		return defaultObservationTimeout
	}
	// use the observation interval as the timeout for all other cases.
	return observationInterval
}

// retrieveHealth returns the current Elasticsearch cluster health
func retrieveHealth(ctx context.Context, cluster types.NamespacedName, esClient esclient.Client) esv1.ElasticsearchHealth {
	log := ulog.FromContext(ctx)
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

// retrieveNodeStats returns the current Elasticsearch node stats
func retrieveNodeStats(ctx context.Context, cluster types.NamespacedName, esClient esclient.Client) esclient.NodesStats {
	log := ulog.FromContext(ctx)
	nodeStats, err := esClient.GetNodesStats(ctx)
	if err != nil {
		log.V(1).Info(
			"Unable to retrieve node stats",
			"error", err,
			"namespace", cluster.Namespace,
			"es_name", cluster.Name,
		)
		return nodeStats
	}
	return nodeStats
}
