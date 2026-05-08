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

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
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
type OnObservation func(cluster types.NamespacedName, previousHealth, newHealth esv1.ElasticsearchHealth)

// consecutiveFailureThreshold is the number of consecutive health check failures required
// before the observer downgrades the cluster health to unknown. This prevents transient
// network issues (e.g., envoy sidecar returning 503 during pod termination) from immediately
// resetting the cluster health and stalling rolling upgrades.
const consecutiveFailureThreshold = 3

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	cluster             types.NamespacedName
	esClient            esclient.Client
	settings            Settings
	creationTime        time.Time
	stopChan            chan struct{}
	stopOnce            sync.Once
	onObservation       OnObservation
	lastHealth          esv1.ElasticsearchHealth
	consecutiveFailures int
	mutex               sync.RWMutex
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
		lastHealth:    esv1.ElasticsearchUnknownHealth, // We don't know the health of the cluster until a first query succeeds
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

// LastHealth returns the last observed state
func (o *Observer) LastHealth() esv1.ElasticsearchHealth {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastHealth
}

// observe retrieves the current ES state, executes onObservation,
// and stores the new state. If the first health check attempt fails, it closes
// idle connections and retries once with a fresh connection. This handles the case
// where the HTTP client has a stale connection to a terminated pod (e.g., behind
// an Istio/envoy sidecar that returns 503 for terminated endpoints).
func (o *Observer) observe(ctx context.Context) {
	defer tracing.Span(&ctx)()
	// Use half the observation interval as per-attempt timeout so the retry
	// has fresh budget. The total time for both attempts stays within the
	// observation interval.
	attemptTimeout := nonNegativeTimeout(o.settings.ObservationInterval) / 2
	if attemptTimeout < 1*time.Second {
		attemptTimeout = 1 * time.Second
	}

	if o.settings.Tracer != nil && apm.TransactionFromContext(ctx) == nil {
		tx := o.settings.Tracer.StartTransaction(name, string(tracing.PeriodicTxType))
		defer tx.End()
		ctx = apm.ContextWithTransaction(ctx, tx)
	}
	// initialise logger after tracing has been started
	ctx = ulog.InitInContext(ctx, name)
	ulog.FromContext(ctx).V(1).Info("Retrieving cluster health", "es_name", o.cluster.Name, "namespace", o.cluster.Namespace)

	// First attempt
	attemptCtx, cancel1 := context.WithTimeout(ctx, attemptTimeout)
	newHealth := retrieveHealth(attemptCtx, o.cluster, o.esClient)
	cancel1()

	// If the first attempt failed, close idle connections to discard any stale
	// TCP connections (e.g., to a terminated pod) and retry once. The URL provider
	// selects a random pod, so the retry will likely reach a different endpoint.
	if newHealth == esv1.ElasticsearchUnknownHealth {
		ulog.FromContext(ctx).V(1).Info("Retrying cluster health after closing idle connections",
			"es_name", o.cluster.Name, "namespace", o.cluster.Namespace)
		o.esClient.CloseIdleConnections()
		attemptCtx2, cancel2 := context.WithTimeout(ctx, attemptTimeout)
		newHealth = retrieveHealth(attemptCtx2, o.cluster, o.esClient)
		cancel2()
	}

	if o.onObservation != nil {
		o.onObservation(o.cluster, o.LastHealth(), newHealth)
	}
	o.updateHealth(newHealth)
}

func (o *Observer) updateHealth(newHealth esv1.ElasticsearchHealth) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if newHealth == esv1.ElasticsearchUnknownHealth {
		o.consecutiveFailures++
		// Keep the previous known health until we've seen enough consecutive failures.
		// This prevents a single transient error (e.g., 503 from envoy during pod termination)
		// from resetting health to unknown and stalling the rolling upgrade.
		if o.consecutiveFailures < consecutiveFailureThreshold && o.lastHealth != esv1.ElasticsearchUnknownHealth {
			log.V(1).Info("Ignoring transient health check failure",
				"consecutiveFailures", o.consecutiveFailures,
				"threshold", consecutiveFailureThreshold,
				"keepingHealth", o.lastHealth,
				"es_name", o.cluster.Name,
				"namespace", o.cluster.Namespace,
			)
			return
		}
	} else {
		o.consecutiveFailures = 0
	}
	o.lastHealth = newHealth
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
