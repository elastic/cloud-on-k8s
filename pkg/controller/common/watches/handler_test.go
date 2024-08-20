// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

type fakeHandler[T client.Object] struct {
	name    string
	handler handler.TypedEventHandler[T, reconcile.Request]
}

func (t fakeHandler[T]) Key() string {
	return t.name
}

func (t fakeHandler[T]) EventHandler() handler.TypedEventHandler[T, reconcile.Request] {
	return t.handler
}

var _ HandlerRegistration[*corev1.Secret] = &fakeHandler[*corev1.Secret]{}

func TestDynamicEnqueueRequest_AddHandler(t *testing.T) {
	tests := []struct {
		name               string
		args               HandlerRegistration[*corev1.Secret]
		wantErr            bool
		registeredHandlers int
	}{
		{
			name:               "registers the given handler with no error",
			args:               &fakeHandler[*corev1.Secret]{},
			wantErr:            false,
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest[*corev1.Secret]()
			if err := d.AddHandler(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("DynamicEnqueueRequest.AddHandler() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, len(d.registrations), tt.registeredHandlers)
		})
	}
}

func TestDynamicEnqueueRequest_RemoveHandler(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(handler *DynamicEnqueueRequest[*corev1.Secret])
		args               HandlerRegistration[*corev1.Secret]
		registeredHandlers int
	}{
		{
			name: "removal on empty handler is a NOOP",
			args: &fakeHandler[*corev1.Secret]{},
		},
		{
			name: "succeed on initialized handler",
			args: &fakeHandler[*corev1.Secret]{},
			setup: func(handler *DynamicEnqueueRequest[*corev1.Secret]) {
				assert.NoError(t, handler.AddHandler(&fakeHandler[*corev1.Secret]{}))
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 0,
		},
		{
			name: "uses key to identify transformer",
			args: &fakeHandler[*corev1.Secret]{
				name: "bar",
			},
			setup: func(handler *DynamicEnqueueRequest[*corev1.Secret]) {
				assert.NoError(t, handler.AddHandler(&fakeHandler[*corev1.Secret]{
					name: "foo",
				}))
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest[*corev1.Secret]()
			if tt.setup != nil {
				tt.setup(d)
			}
			d.RemoveHandler(tt.args)
			assert.Equal(t, len(d.registrations), tt.registeredHandlers)
		})
	}
}

func TestDynamicEnqueueRequest_EventHandler(t *testing.T) {
	// Fixtures
	nsn1 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched1",
	}
	testObject1 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(nsn1),
	}
	updated1 := testObject1
	updated1.Labels = map[string]string{"updated": "1"}

	nsn2 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched2",
	}
	testObject2 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(nsn2),
	}
	updated2 := testObject2
	updated2.Labels = map[string]string{"updated": "2"}

	watching := types.NamespacedName{
		Namespace: "default",
		Name:      "watcher",
	}

	d := NewDynamicEnqueueRequest[*corev1.Secret]()
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	assertEmptyQueue := func() {
		require.Equal(t, 0, q.Len())
	}
	getReconcileReqFromQueue := func() reconcile.Request {
		item, shutdown := q.Get()
		defer q.Done(item)
		require.False(t, shutdown)
		return item
	}
	assertReconcileReq := func(nsn types.NamespacedName) {
		require.Equal(t, getReconcileReqFromQueue().NamespacedName, nsn)
	}

	assertEmptyQueue()

	// simulate an object creation
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject1,
	}, q)
	assertEmptyQueue()

	// Add a watch for the first object
	require.NoError(t, d.AddHandler(NamedWatch[*corev1.Secret]{
		Watched: []types.NamespacedName{nsn1},
		Watcher: watching,
		Name:    "test-watch-1",
	}))
	assertEmptyQueue()

	// simulate first object creation
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject1,
	}, q)
	assertReconcileReq(watching)

	// simulate object update
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject1,
		ObjectNew: updated1,
	}, q)
	assertReconcileReq(watching)
	// simulate object deletion
	d.Delete(context.Background(), event.TypedDeleteEvent[*corev1.Secret]{
		Object: testObject1,
	}, q)
	assertReconcileReq(watching)

	// simulate second object creation
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject2,
	}, q)
	// no watcher, nothing in the queue
	assertEmptyQueue()
	// simulate second object update
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	// no watcher, nothing in the queue
	assertEmptyQueue()

	// register a second watch for the second object
	require.NoError(t, d.AddHandler(NamedWatch[*corev1.Secret]{
		Watched: []types.NamespacedName{nsn2},
		Watcher: watching,
		Name:    "test-watch-2",
	}))
	// simulate second object creation
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject2,
	}, q)
	assertReconcileReq(watching)
	// simulate second object update
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(watching)

	// remove the watch for object 2
	d.RemoveHandlerForKey("test-watch-2")
	// simulate object update: nothing should happen
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	assertEmptyQueue()

	// updates on the first object should still work
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject1,
		ObjectNew: updated1,
	}, q)
	assertReconcileReq(watching)

	// let's combine both objects in a single watch
	require.NoError(t, d.AddHandler(NamedWatch[*corev1.Secret]{
		Name:    "test-watch-1",
		Watched: []types.NamespacedName{nsn1, nsn2},
		Watcher: watching,
	}))

	// update on the first object should register
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject1,
		ObjectNew: updated1,
	}, q)
	assertReconcileReq(watching)

	// update on the second object should register too
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(watching)

	// going back to watching object 1 only
	require.NoError(t, d.AddHandler(NamedWatch[*corev1.Secret]{
		Watched: []types.NamespacedName{nsn1},
		Watcher: watching,
		Name:    "test-watch-1",
	}))
	assertEmptyQueue()

	// setup an owner watch where owner is testObject1
	require.NoError(t, d.AddHandler(&OwnerWatch[*corev1.Secret]{
		Scheme:       scheme.Scheme,
		Mapper:       getRESTMapper(),
		OwnerType:    testObject1,
		IsController: true,
	}))

	// let's make object 1 the owner of object 2
	require.NoError(t, controllerutil.SetControllerReference(testObject1, testObject2, scheme.Scheme))
	// an update on object 2 should enqueue a request for object 1 (the owner)
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(nsn1)
	// same for deletes
	d.Delete(context.Background(), event.TypedDeleteEvent[*corev1.Secret]{
		Object: testObject2,
	}, q)
	assertReconcileReq(nsn1)

	// named watch on object 1 should still work
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject1,
	}, q)
	assertReconcileReq(watching)

	// it's possible to have both an owner watch and a named watch triggered
	// for a single event
	// add a named watch on object 2
	require.NoError(t, d.AddHandler(NamedWatch[*corev1.Secret]{
		Watched: []types.NamespacedName{nsn2},
		Watcher: watching,
		Name:    "test-watch-2",
	}))
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject2,
	}, q)
	expected := []types.NamespacedName{
		// owner watch (for object1) should trigger since object2's owner is object1
		nsn1,
		// named watch (for object2) should trigger since object2 was updated
		watching,
	}
	// actual order is non-deterministic
	req1 := getReconcileReqFromQueue()
	req2 := getReconcileReqFromQueue()
	require.ElementsMatch(t, expected, []types.NamespacedName{req1.NamespacedName, req2.NamespacedName})
}

func TestDynamicEnqueueRequest_OwnerWatch(t *testing.T) {
	// Fixtures
	nsn1 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched1",
	}
	testObject1 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(nsn1),
	}
	updated1 := testObject1
	updated1.Labels = map[string]string{"updated": "1"}

	nsn2 := types.NamespacedName{
		Namespace: "default",
		Name:      "watched2",
	}
	testObject2 := &corev1.Secret{
		ObjectMeta: k8s.ToObjectMeta(nsn2),
	}
	updated2 := testObject2
	updated2.Labels = map[string]string{"updated": "2"}

	d := NewDynamicEnqueueRequest[*corev1.Secret]()
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())

	assertEmptyQueue := func() {
		require.Equal(t, 0, q.Len())
	}
	getReconcileReqFromQueue := func() reconcile.Request {
		item, shutdown := q.Get()
		defer q.Done(item)
		require.False(t, shutdown)
		return item
	}
	assertReconcileReq := func(nsn types.NamespacedName) {
		require.Equal(t, getReconcileReqFromQueue().NamespacedName, nsn)
	}

	assertEmptyQueue()
	// setup an owner watch where owner is testObject1
	require.NoError(t, d.AddHandler(&OwnerWatch[*corev1.Secret]{
		OwnerType:    testObject1,
		IsController: true,
		Scheme:       scheme.Scheme,
		Mapper:       getRESTMapper(),
	}))
	// END FIXTURES

	require.NoError(t, controllerutil.SetControllerReference(testObject1, testObject2, scheme.Scheme))

	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject1,
	}, q)
	d.Create(context.Background(), event.TypedCreateEvent[*corev1.Secret]{
		Object: testObject2,
	}, q)

	// an update on object 2 should enqueue a request for object 1 (the owner)
	d.Update(context.Background(), event.TypedUpdateEvent[*corev1.Secret]{
		ObjectOld: testObject2,
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(nsn1)
}

// getRESTMapper returns a RESTMapper used to inject a mapper into a dynamic queue request
func getRESTMapper() meta.RESTMapper {
	resources := []*restmapper.APIGroupResources{
		{
			Group: metav1.APIGroup{
				Versions: []metav1.GroupVersionForDiscovery{
					{Version: "v1"},
				},
				PreferredVersion: metav1.GroupVersionForDiscovery{Version: "v1"},
			},
			VersionedResources: map[string][]metav1.APIResource{
				"v1": {
					{Name: "secrets", Namespaced: true, Kind: "Secret"},
				},
			},
		},
	}

	return restmapper.NewDiscoveryRESTMapper(resources)
}
