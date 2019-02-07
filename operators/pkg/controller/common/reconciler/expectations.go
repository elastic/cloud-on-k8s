// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

// Expectations are a way for controllers to mitigate effects of
// the K8s client cache lagging behind the apiserver.
//
// ## Context: client cache might be out-of-sync
//
// The default K8s client implementation does use a cache for all resources we get or list.
// Listing pods effectively returns pods that have been observed in the cache, relying on a
// watch being set up by the client behind the scenes.
// Hence, resources we get from a list operation may slightly lag behind resources in the apiserver.
// The cache is not invalidated on resource creation. The following can happen in a controller:
//
// * list pods: we get 2
// * create a new pod
// * list pods again: we still get 2 (cache in not in sync yet)
//
// This could lead to creating the pod a second time (with a different generated name) at the
// next iteration of the reconciliation loop.
// The same goes for deletions.
//
// This is only a problem for resources whose name is non-deterministic. Creating twice the same
// resource with the same name is considered OK, since the second time would simply fail.
//
// ## Expectations as a solution to mitigate cache inconsistencies
//
// ReplicaSets implementation in K8s does rely on runtime expectations to mitigate those inconsistencies.
// See the expectations implementation: https://github.com/kubernetes/kubernetes/blob/v1.13.2/pkg/controller/controller_utils.go
// And its usage for ReplicaSets: https://github.com/kubernetes/kubernetes/blob/v1.13.2/pkg/controller/replicaset/replica_set.go#L90
//
// The idea is the following:
//
// * When a resource is created, increase the expected creations for this resource.
//   Example: "expect 1 pod creation for this ElasticsearchCluster". Note that expectations
//   are associated to the ElasticsearchCluster resource here, but effectively observe pods.
// * Once the resource creation event is observed, decrease the expected creations (expectation observed).
// * Expectations are considered satisfied when the count is equal to 0: we can consider our cache in-sync.
// * Checking whether expectations are satisfied within a reconciliation loop iteration is a way to know
//   whether we can move forward with an up-to-date cache to next reconciliation steps.
// * The same goes for deletions.
//
// Expectations have a time-to-live (5 minutes). Once reached, we consider an expectation to be fulfilled, even
// though its internal counters may not be 0. This is to avoid staying stuck with inconsistent expectation events.
//
// ## Why not reusing K8s expectations implementations?
//
// We could absolutely reuse the existing `controller.Expectations` implementations.
// Doing so forces us to vendor the whole `kubernetes` package tree, which in turns
// requires vendoring the apiserver package tree. That's a lot of imports.
//
// Also, the Expectations API is not very user-friendly.
//
// A common usage in our situation is to increment expectations whenever we create a pod.
// Two ways to do that with K8s Expectations API:
//
// * `expectations.ExpectCreations(controllerKey string, adds int)`: overrides any previous value.
// * `expectations.RaiseExpectations(controllerKey string, add, del int)`: only works if expectations exist,
//   meaning `expectations.SetExpectations was called at least once before.
//
// This is replaced in our implementation by a simpler `expectations.ExpectCreations(controllerKey)`,
// that does increment the creation counter, and creates it if it doesn't exist yet.
//
// A few other things that differ in our implementation from the K8s one:
//
// * We don't accept negative counters as a correct value: it does not make sense to set the creations
//   counter to -1 if it was already at 0 (could be a leftover creation from a previous controller that
//   we don't care about, since we don't have expectations for it).
// * Once an expectations TTL is reached, we consider we probably missed an event, hence we choose to
//   reset expectations to 0 explicitely, instead of keeping counters value but still consider expectations
//   to be fulfilled.
// * `controller.UIDTrackingControllerExpectations` is an extended expectations implementation meant to handle
//   update events that have a non-zero DeletionTimestamp (can be issued multiple times but should be counted
//   only once). Since we do rely on controller-runtime deletion events instead, but don't need this here.
// * We only use atomic int64 here, no generic `cache.Store`: no need to deal with error handling in the caller.
//
// ## Usage
//
// Expected usage pseudo-code:
// ```
// if !expectations.fulfilled(responsibleResourceID) {
//     // expected creations and deletions are not fulfilled yet,
//     // let's requeue
//     return
// }
// for _, res := range resourcesToCreate {
//     // expect a creation
//     expectations.ExpectCreation(responsibleResourceID)
//     if err := client.Create(res); err != nil {
//	       // cancel our expectation, since resource wasn't created
//         expectations.CreationObserved(responsibleResourceID)
//         return err
//     }
// }
// // same mechanism for deletions
// ```
//
// Note that the `responsibleResourceID` in this context does not map to resources we create
// or delete. For instance, it would be the ID of our ElasticsearchCluster, even though the
// resources that we effectively create and deletes are pods associated with this cluster.
//

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// ExpectationsTTLNanosec is the default expectations time-to-live,
	// for cases where we expect an event (creation or deletion) that never happens.
	//
	// Set to 5 minutes similar to https://github.com/kubernetes/kubernetes/blob/v1.13.2/pkg/controller/controller_utils.go
	ExpectationsTTLNanosec = 5 * time.Minute // time is internally represented as int64 nanoseconds

	// ExpectationsFinalizerName designates a finalizer to clean up expectations on es cluster deletion.
	ExpectationsFinalizerName = "expectations.finalizers.elasticsearch.stack.k8s.elastic.co"
)

// NewExpectations creates expectations with the default TTL.
func NewExpectations() *Expectations {
	return &Expectations{
		mutex:    sync.RWMutex{},
		counters: map[types.NamespacedName]*expectationsCounters{},
		ttl:      ExpectationsTTLNanosec,
	}
}

// Expectations holds our creation and deletions expectations for
// various resources, up to the configured TTL.
// Safe for concurrent use.
type Expectations struct {
	mutex    sync.RWMutex
	counters map[types.NamespacedName]*expectationsCounters
	ttl      time.Duration
}

// ExpectCreation marks a creation for the given resource as expected.
func (e *Expectations) ExpectCreation(namespacedName types.NamespacedName) {
	e.getOrCreateCounters(namespacedName).AddCreations(1)
}

// ExpectDeletion marks a deletion for the given resource as expected.
func (e *Expectations) ExpectDeletion(namespacedName types.NamespacedName) {
	e.getOrCreateCounters(namespacedName).AddDeletions(1)
}

// CreationObserved marks a creation event for the given resource as observed,
// cancelling the effect of a previous call to e.ExpectCreation.
func (e *Expectations) CreationObserved(namespacedName types.NamespacedName) {
	e.getOrCreateCounters(namespacedName).AddCreations(-1)
}

// DeletionObserved marks a deletion event for the given resource as observed,
// cancelling the effect of a previous call to e.ExpectDeletion.
func (e *Expectations) DeletionObserved(namespacedName types.NamespacedName) {
	e.getOrCreateCounters(namespacedName).AddDeletions(-1)
}

// Fulfilled returns true if all the expectations for the given resource
// are fulfilled (both creations and deletions). Meaning we can consider
// the controller is in-sync with resources in the apiserver.
func (e *Expectations) Fulfilled(namespacedName types.NamespacedName) bool {
	creations, deletions := e.get(namespacedName)
	if creations == 0 && deletions == 0 {
		return true
	}
	return false
}

// get creations and deletions expectations for the expected resource.
func (e *Expectations) get(namespacedName types.NamespacedName) (creations int64, deletions int64) {
	return e.getOrCreateCounters(namespacedName).Get()
}

// getOrCreateCounters returns the counters associated to the given resource.
// They may not exist yet: in such case we create and initialize them first.
func (e *Expectations) getOrCreateCounters(namespacedName types.NamespacedName) *expectationsCounters {
	e.mutex.RLock()
	counters, exists := e.counters[namespacedName]
	e.mutex.RUnlock()
	if !exists {
		counters = e.createCounters(namespacedName)
	}
	return counters
}

func (e *Expectations) createCounters(namespacedName types.NamespacedName) *expectationsCounters {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	// if this method is called, counters probably don't exist yet
	// still re-check with lock acquired in case they would be created
	// in-between 2 concurrent calls to e.getOrCreateCounters
	counters, exists := e.counters[namespacedName]
	if exists {
		return counters
	}
	counters = newExpectationsCounters(e.ttl)
	e.counters[namespacedName] = counters
	return counters
}

// expectationsCounters hold creations and deletions counters,
// and manages counters TTL through their last activity timestamp.
// Counters that would go below 0 will be reset to 0.
// Expectations that would exceed their TTL will be reset to 0.
// Safe for concurrent use.
type expectationsCounters struct {
	creations *int64 // atomic int64 counter
	deletions *int64 // atomic int64 counter
	timestamp *int64 // unix timestamp in nanoseconds
	ttl       int64  // duration in nanoseconds
}

// newExpectationsCounters returns an initiliazed expectationsCounters
// with the given ttl, and timestamp set to now.
func newExpectationsCounters(ttl time.Duration) *expectationsCounters {
	creations := int64(0)
	deletions := int64(0)
	timestamp := timestampNow()
	return &expectationsCounters{
		creations: &creations,
		deletions: &deletions,
		timestamp: &timestamp,
		ttl:       ttl.Nanoseconds(),
	}
}

// Get returns the current creations and deletions counters.
// If counters are expired, they are reset to 0 beforehand.
func (e *expectationsCounters) Get() (creations, deletions int64) {
	if e.isExpired() {
		e.reset()
	}
	return e.getPtrValue(e.creations), e.getPtrValue(e.deletions)
}

// AddCreations increments the creations counter with the given value,
// which can be negative for substractions.
// If the value goes below 0, it will be reset to 0.
func (e *expectationsCounters) AddCreations(value int64) {
	e.add(e.creations, value)
}

// AddDeletions increments the deletions counter with the given value,
// which can be negative for substractions.
// If the value goes below 0, it will be reset to 0.
func (e *expectationsCounters) AddDeletions(value int64) {
	e.add(e.deletions, value)
}

// isExpired returns true if the last operation on the counters
// exceeds the configured TTL.
func (e *expectationsCounters) isExpired() bool {
	if e.timestamp == nil {
		return false
	}
	timestamp := atomic.LoadInt64(e.timestamp)
	if timestampNow()-timestamp > e.ttl {
		return true
	}
	return false
}

// resetTimestamp sets the timestamp value to the current time.
func (e *expectationsCounters) resetTimestamp() {
	atomic.StoreInt64(e.timestamp, timestampNow())
}

// reset sets counters values to 0, and the
// timestamp value to the current time.
func (e *expectationsCounters) reset() {
	atomic.StoreInt64(e.creations, 0)
	atomic.StoreInt64(e.deletions, 0)
	e.resetTimestamp()
}

// getPtrValue returns the int64 value stored at the given pointer.
// Meant to be used for internal values, eg. `getPtrValue(e.creations)`.
func (e *expectationsCounters) getPtrValue(ptr *int64) int64 {
	value := atomic.LoadInt64(ptr)
	if value < 0 {
		// In-between situation where we have a negative value,
		// return 0 instead (see `e.add` implementation).
		return 0
	}
	return value
}

// add increments the int64 stored at the given pointer with the given value,
// which can be negative for substractions.
// Meant to be used for internal values, eg. `add(e.creations, -1)`.
// If the value goes below 0, it will be reset to 0.
func (e *expectationsCounters) add(ptr *int64, value int64) {
	e.resetTimestamp()
	newValue := atomic.AddInt64(ptr, value)
	if newValue < 0 && value < 0 {
		// We are reaching a negative value after a substraction:
		// cancel what we just did.
		// The value is still negative in-between these 2 atomic ops.
		atomic.AddInt64(ptr, -value)
	}
}

// timestampNow returns the current unix timestamp in nanoseconds
func timestampNow() int64 {
	return time.Now().UnixNano()
}

// ExpectationsFinalizer removes the given cluster entry from the expectations map.
func ExpectationsFinalizer(cluster types.NamespacedName, expectations *Expectations) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: ExpectationsFinalizerName,
		Execute: func() error {
			expectations.mutex.Lock()
			defer expectations.mutex.Unlock()
			delete(expectations.counters, cluster)
			return nil
		},
	}
}
