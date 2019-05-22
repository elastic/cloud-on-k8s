// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package watches

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func TestMain(m *testing.M) {
	apis.AddToScheme(scheme.Scheme) // here to avoid import cycle
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "..", "config", "crds"))
}

// SetupTestWatch sets up a reconcile.Reconcile with the given watch that
// writes any reconcile requests to requests.
func SetupTestWatch(t *testing.T, source source.Source, handler handler.EventHandler) (manager.Manager, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		requests <- req
		return reconcile.Result{}, nil
	})

	mgr, err := manager.New(test.Config, manager.Options{})
	assert.NoError(t, err)

	ctrl, err := controller.New("test-reconciler", mgr, controller.Options{Reconciler: fn})
	assert.NoError(t, err)

	assert.NoError(t, ctrl.Watch(source, handler))
	return mgr, requests
}

// StartTestManager starts the controller manager and all controllers.
func StartTestManager(mgr manager.Manager, t *testing.T) (chan struct{}, *sync.WaitGroup) {
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := mgr.Start(stop)
		assert.NoError(t, err)
		wg.Done()
	}()
	return stop, wg
}

// TestDynamicEnqueueRequest tests all operations on dynamic watches in one big test to minimize time lost during
// bootstrapping the test environment.
func TestDynamicEnqueueRequest(t *testing.T) {
	// Fixtures
	watched1 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched1",
	}
	watched2 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched2",
	}

	testObject1 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(watched1),
	}

	testObject2 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(watched2),
	}

	watching := types.NamespacedName{
		Namespace: "default",
		Name:      "watcher",
	}

	watcherReconcileRequest := reconcile.Request{
		NamespacedName: watching,
	}
	// Watch + Controller setup
	src := &source.Kind{Type: &corev1.Secret{}}
	eventHandler := NewDynamicEnqueueRequest()
	mgr, requests := SetupTestWatch(t, src, eventHandler)
	stopMgr, mgrStopped := StartTestManager(mgr, t)

	oneSecond := 1 * time.Second

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	c := k8s.WrapClient(mgr.GetClient())

	// Create the first object before registering any watches
	assert.NoError(t, c.Create(testObject1))

	// Expect no reconcile requests as we don't have registered the watch yet
	CheckReconcileNotCalledWithin(t, requests, oneSecond)

	// Add a watch for the first object
	assert.NoError(t, eventHandler.AddHandler(NamedWatch{
		Watched: watched1,
		Watcher: watching,
		Name:    "test-watch-1",
	}))

	// Update the first object and expect a reconcile request
	testLabels := map[string]string{"test": "label"}
	testObject1.Labels = testLabels
	assert.NoError(t, c.Update(testObject1))
	CheckReconcileCalledIn(t, requests, watcherReconcileRequest, 1, 2)

	// Now register a second watch for the other object
	watch := NamedWatch{
		Watched: watched2,
		Watcher: watching,
		Name:    "test-watch-2",
	}
	assert.NoError(t, eventHandler.AddHandler(watch))
	// ... and create the second object and expect a corresponding reconcile request
	assert.NoError(t, c.Create(testObject2))
	CheckReconcileCalled(t, requests, watcherReconcileRequest)

	// Remove the watch for object 1 again
	eventHandler.RemoveHandlerForKey("test-watch-1")
	// trigger another update but don't expect any requests as we have unregistered the watch
	assert.NoError(t, c.Update(testObject1))
	CheckReconcileNotCalledWithin(t, requests, oneSecond)

	// The second watch should still work
	testObject2.Labels = testLabels
	assert.NoError(t, c.Update(testObject2))
	// Depending on the scheduling of the test execution the two reconcile.Requests might be coalesced into one
	CheckReconcileCalledIn(t, requests, watcherReconcileRequest, 1, 2)

	// Until we remove it
	eventHandler.RemoveHandler(watch)
	// update object 2 again and don't expect a request
	testObject2.Labels = map[string]string{}
	assert.NoError(t, c.Update(testObject2))
	CheckReconcileNotCalledWithin(t, requests, oneSecond)

	// Owner watches should work as before
	ownerWatch := &OwnerWatch{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			OwnerType:    testObject2,
			IsController: true,
		},
	}
	assert.NoError(t, eventHandler.AddHandler(ownerWatch))

	// Let's make object 2 the owner of object 1
	assert.NoError(t, controllerutil.SetControllerReference(testObject2, testObject1, scheme.Scheme))
	assert.NoError(t, c.Update(testObject1))
	// Depending on the scheduling of the test execution the two reconcile.Requests might be coalesced into one
	CheckReconcileCalledIn(t, requests, reconcile.Request{NamespacedName: watched2}, 1, 2)

	// We should be able to use both labeled watches and owner watches
	assert.NoError(t, eventHandler.AddHandler(watch))
	testObject2.Labels = testLabels
	assert.NoError(t, c.Update(testObject2))
	// Depending on the scheduling of the test execution the two reconcile.Requests might be coalesced into one
	CheckReconcileCalledIn(t, requests, watcherReconcileRequest, 1, 2)

	// Delete requests should be observable as well
	assert.NoError(t, c.Delete(testObject1))
	CheckReconcileCalled(t, requests, reconcile.Request{NamespacedName: watched2})
	assert.NoError(t, c.Delete(testObject2))
	CheckReconcileCalled(t, requests, watcherReconcileRequest)
}

// CheckReconcileCalledIn waits up to Timeout to receive the expected request on requests.
func CheckReconcileCalledIn(t *testing.T, requests chan reconcile.Request, expected reconcile.Request, min, max int) {
	var seen int
	for seen < max {
		select {
		case req := <-requests:
			seen++
			assert.Equal(t, req, expected)
		case <-time.After(test.Timeout / time.Duration(max)):
			if seen < min {
				assert.Fail(t, fmt.Sprintf("No request received after %s", test.Timeout))
			}
			return
		}
	}
}

// CheckReconcileCalled waits up to Timeout to receive the expected request on requests.
func CheckReconcileCalled(t *testing.T, requests chan reconcile.Request, expected reconcile.Request) {
	select {
	case req := <-requests:
		assert.Equal(t, expected, req)
	case <-time.After(test.Timeout):
		assert.Fail(t, fmt.Sprintf("No request received after %s", test.Timeout))
	}
}

// CheckReconcileNotCalled ensures that no reconcile requests are currently pending
func CheckReconcileNotCalledWithin(t *testing.T, requests chan reconcile.Request, duration time.Duration) {
	select {
	case req := <-requests:
		assert.Fail(t, fmt.Sprintf("No request expected but got %v", req))
	case <-time.After(duration):
		//no request received, OK moving on
	}
}
