package observer

import (
	"context"
	"sync"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
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

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	cluster  types.NamespacedName
	esClient *client.Client

	settings Settings

	creationTime time.Time

	stopChan chan struct{}
	stopOnce sync.Once

	lastObservationTime time.Time
	lastState           State
	mutex               sync.RWMutex
}

// NewObserver creates and starts an Observer
func NewObserver(cluster types.NamespacedName, esClient *client.Client, settings Settings) *Observer {
	observer := Observer{
		cluster:      cluster,
		esClient:     esClient,
		creationTime: time.Now(),
		settings:     settings,
		stopChan:     make(chan struct{}),
		stopOnce:     sync.Once{},
		mutex:        sync.RWMutex{},
	}
	log.Info("Creating observer", "cluster", cluster)
	go observer.run()
	return &observer
}

// Stop the observer loop
func (o *Observer) Stop() {
	// trigger an async stop, only once
	o.stopOnce.Do(func() {
		go func() {
			close(o.stopChan)
		}()
	})
}

// LastState returns the last observed state
func (o *Observer) LastState() State {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastState
}

// LastObservationTime returns the time of the last observation
func (o *Observer) LastObservationTime() time.Time {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	return o.lastObservationTime
}

// run the observer main loop, until stopped
func (o *Observer) run() {
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
			log.Info("Stopping observer", "cluster", o.cluster)
			return
		}
	}
}

// retrieveState retrieves the current ES state and stores it in lastState
func (o *Observer) retrieveState(ctx context.Context) {
	log.V(4).Info("Retrieving state", "cluster", o.cluster)
	timeoutCtx, cancel := context.WithTimeout(ctx, o.settings.RequestTimeout)
	defer cancel()
	state := RetrieveState(timeoutCtx, o.esClient)
	o.mutex.Lock()
	o.lastState = state
	o.lastObservationTime = time.Now()
	o.mutex.Unlock()
}
