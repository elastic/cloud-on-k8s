// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/processmanager"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	netutils "github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("observer")

// Settings for the Observer configuration
type Settings struct {
	ObservationInterval time.Duration
	RequestTimeout      time.Duration
}

// Default values:
// - best-case scenario (healthy cluster): a request is performed every 10 seconds
// - worst-case scenario (unhealthy cluster): a request is performed every 70 (60+10) seconds
const (
	DefaultObservationInterval = 10 * time.Second
	DefaultRequestTimeout      = 1 * time.Minute
)

// DefaultSettings is an observer's Params with default values
var DefaultSettings = Settings{
	ObservationInterval: DefaultObservationInterval,
	RequestTimeout:      DefaultRequestTimeout,
}

// OnObservation is a function that gets executed when a new state is observed
type OnObservation func(cluster types.NamespacedName, previousState State, newState State)

// pmClientFactory is a function to create process manager client (to ease testing)
type pmClientFactory func(pod corev1.Pod) processmanager.Client

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	k8sClient       k8s.Client
	dialer          netutils.Dialer
	caCerts         []*x509.Certificate
	pmClientFactory pmClientFactory
	cluster         types.NamespacedName
	esClient        client.Client

	settings Settings

	creationTime time.Time

	stopChan chan struct{}
	stopOnce sync.Once

	onObservation OnObservation

	lastState State
	mutex     sync.RWMutex
}

// NewObserver creates and starts an Observer
func NewObserver(
	k8sClient k8s.Client, dialer netutils.Dialer, caCerts []*x509.Certificate,
	cluster types.NamespacedName, esClient client.Client,
	settings Settings, onObservation OnObservation,
) *Observer {
	observer := Observer{
		k8sClient:     k8sClient,
		dialer:        dialer,
		caCerts:       caCerts,
		cluster:       cluster,
		esClient:      esClient,
		creationTime:  time.Now(),
		settings:      settings,
		stopChan:      make(chan struct{}),
		stopOnce:      sync.Once{},
		onObservation: onObservation,
		mutex:         sync.RWMutex{},
	}
	observer.pmClientFactory = observer.createProcessManagerClient

	log.Info("Creating observer for cluster", "namespace", cluster.Namespace, "es_name", cluster.Name)
	return &observer
}

// Start the observer in a separate goroutine
func (o *Observer) Start() {
	go o.runUntilStopped()
}

// Stop the observer loop
func (o *Observer) Stop() {
	// trigger an async stop, only once
	o.stopOnce.Do(func() {
		go func() {
			close(o.stopChan)
			o.esClient.Close()
		}()
	})
}

// LastState returns the last observed state
func (o *Observer) LastState() State {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastState
}

// run the observer main loop, until stopped
func (o *Observer) runUntilStopped() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go o.runPeriodically(ctx)
	<-o.stopChan
}

// runPeriodically triggers a state retrieval every tick,
// until the given context is cancelled
func (o *Observer) runPeriodically(ctx context.Context) {
	o.retrieveState(ctx)
	ticker := time.NewTicker(o.settings.ObservationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			o.retrieveState(ctx)
		case <-ctx.Done():
			log.Info("Stopping observer for cluster", "namespace", o.cluster.Namespace, "es_name", o.cluster.Name)
			return
		}
	}
}

// retrieveState retrieves the current ES state, executes onObservation,
// and stores the new state
func (o *Observer) retrieveState(ctx context.Context) {
	log.V(1).Info("Retrieving cluster state", "es_name", o.cluster.Name, "namespace", o.cluster.Namespace)
	timeoutCtx, cancel := context.WithTimeout(ctx, o.settings.RequestTimeout)
	defer cancel()

	newState := RetrieveState(timeoutCtx, o.cluster, o.esClient, o.k8sClient, o.pmClientFactory)

	if o.onObservation != nil {
		o.onObservation(o.cluster, o.LastState(), newState)
	}

	o.mutex.Lock()
	o.lastState = newState
	o.mutex.Unlock()
}

func (o *Observer) createProcessManagerClient(pod corev1.Pod) processmanager.Client {
	podIP := net.ParseIP(pod.Status.PodIP)
	url := fmt.Sprintf("https://%s:%d", podIP.String(), processmanager.DefaultPort)
	return processmanager.NewClient(url, o.caCerts, o.dialer)
}
