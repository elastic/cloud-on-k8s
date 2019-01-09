package observer

import (
	"sync"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
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

// DefaultSettings is an observer Params with default values
var DefaultSettings = Settings{
	ObservationInterval: DefaultObservationInterval,
	RequestTimeout:      DefaultRequestTimeout,
}

// Observer regularly requests an ES endpoint for cluster state,
// in a thread-safe way
type Observer struct {
	clusterName string
	esClient    *client.Client

	settings Settings

	creationTime time.Time

	stopChan chan struct{}
	stopOnce sync.Once

	lastObservationTime time.Time
	lastState           State
	mutex               sync.RWMutex
}

// NewObserver creates and starts an Observer
func NewObserver(clusterName string, esClient *client.Client, settings Settings) *Observer {
	observer := Observer{
		clusterName:  clusterName,
		esClient:     esClient,
		creationTime: time.Now(),
		settings:     settings,
		stopChan:     make(chan struct{}),
		stopOnce:     sync.Once{},
		mutex:        sync.RWMutex{},
	}
	log.Info("Creating observer", "clusterName", clusterName)
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
	o.retrieveState()
	ticker := time.NewTicker(o.settings.ObservationInterval)
	for {
		select {
		case <-o.stopChan:
			log.Info("Stopping observer", "clusterName", o.clusterName)
			ticker.Stop()
			return
		case <-ticker.C:
			o.retrieveState()
		}
	}
}

// retrieveState retrieves the current ES state and stores it in lastState
func (o *Observer) retrieveState() {
	log.V(4).Info("Retrieving state", "clusterName", o.clusterName)
	state := RetrieveState(o.esClient, o.settings.RequestTimeout)
	o.mutex.Lock()
	o.lastState = state
	o.lastObservationTime = time.Now()
	o.mutex.Unlock()
}
