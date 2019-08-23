// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type fakeHandler struct {
	name    string
	handler handler.EventHandler
}

func (t fakeHandler) Key() string {
	return t.name
}

func (t fakeHandler) EventHandler() handler.EventHandler {
	return t.handler
}

var _ HandlerRegistration = &fakeHandler{}

func TestDynamicEnqueueRequest_AddHandler(t *testing.T) {
	tests := []struct {
		name               string
		setup              func(handler *DynamicEnqueueRequest)
		args               HandlerRegistration
		wantErr            bool
		registeredHandlers int
	}{
		{
			name:    "fail on uninitialized handler",
			args:    &fakeHandler{},
			wantErr: true,
		},
		{
			name: "succeed on initialized handler",
			args: &fakeHandler{},
			setup: func(handler *DynamicEnqueueRequest) {
				assert.NoError(t, handler.InjectScheme(scheme.Scheme))
			},
			wantErr:            false,
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
			if tt.setup != nil {
				tt.setup(d)
			}
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
		setup              func(handler *DynamicEnqueueRequest)
		args               HandlerRegistration
		registeredHandlers int
	}{
		{
			name: "removal on empty handler is a NOOP",
			args: &fakeHandler{},
		},
		{
			name: "succeed on initialized handler",
			args: &fakeHandler{},
			setup: func(handler *DynamicEnqueueRequest) {
				assert.NoError(t, handler.InjectScheme(scheme.Scheme))
				assert.NoError(t, handler.AddHandler(&fakeHandler{}))
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 0,
		},
		{
			name: "uses key to identify transformer",
			args: &fakeHandler{
				name: "bar",
			},
			setup: func(handler *DynamicEnqueueRequest) {
				assert.NoError(t, handler.InjectScheme(scheme.Scheme))
				assert.NoError(t, handler.AddHandler(&fakeHandler{
					name: "foo",
				}))
				assert.Equal(t, len(handler.registrations), 1)
			},
			registeredHandlers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDynamicEnqueueRequest()
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

	d := NewDynamicEnqueueRequest()
	require.NoError(t, d.InjectScheme(scheme.Scheme))
	q := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	assertEmptyQueue := func() {
		require.Equal(t, 0, q.Len())
	}
	getReconcileReqFromQueue := func() reconcile.Request {
		item, shutdown := q.Get()
		defer q.Done(item)
		require.False(t, shutdown)
		req, ok := item.(reconcile.Request)
		require.True(t, ok)
		return req
	}
	assertReconcileReq := func(nsn types.NamespacedName) {
		require.Equal(t, getReconcileReqFromQueue().NamespacedName, nsn)
	}

	assertEmptyQueue()

	// simulate an object creation
	d.Create(event.CreateEvent{
		Object: testObject1,
		Meta:   testObject1.GetObjectMeta(),
	}, q)
	assertEmptyQueue()

	// Add a watch for the first object
	require.NoError(t, d.AddHandler(NamedWatch{
		Watched: nsn1,
		Watcher: watching,
		Name:    "test-watch-1",
	}))
	assertEmptyQueue()

	// simulate first object creation
	d.Create(event.CreateEvent{
		Object: testObject1,
		Meta:   testObject1.GetObjectMeta(),
	}, q)
	assertReconcileReq(watching)

	// simulate object update
	d.Update(event.UpdateEvent{
		MetaOld:   testObject1.GetObjectMeta(),
		ObjectOld: testObject1,
		MetaNew:   updated1.GetObjectMeta(),
		ObjectNew: updated1,
	}, q)
	assertReconcileReq(watching)
	// simulate object deletion
	d.Delete(event.DeleteEvent{
		Object: testObject1,
		Meta:   testObject1.GetObjectMeta(),
	}, q)
	assertReconcileReq(watching)

	// simulate second object creation
	d.Create(event.CreateEvent{
		Object: testObject2,
		Meta:   testObject2.GetObjectMeta(),
	}, q)
	// no watcher, nothing in the queue
	assertEmptyQueue()
	// simulate second object update
	d.Update(event.UpdateEvent{
		MetaOld:   testObject2.GetObjectMeta(),
		ObjectOld: testObject2,
		MetaNew:   updated2.GetObjectMeta(),
		ObjectNew: updated2,
	}, q)
	// no watcher, nothing in the queue
	assertEmptyQueue()

	// register a second watch for the second object
	require.NoError(t, d.AddHandler(NamedWatch{
		Watched: nsn2,
		Watcher: watching,
		Name:    "test-watch-2",
	}))
	// simulate second object creation
	d.Create(event.CreateEvent{
		Object: testObject2,
		Meta:   testObject2.GetObjectMeta(),
	}, q)
	assertReconcileReq(watching)
	// simulate second object update
	d.Update(event.UpdateEvent{
		MetaOld:   testObject2.GetObjectMeta(),
		ObjectOld: testObject2,
		MetaNew:   updated2.GetObjectMeta(),
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(watching)

	// remove the watch for object 2
	d.RemoveHandlerForKey("test-watch-2")
	// simulate object update: nothing should happen
	d.Update(event.UpdateEvent{
		MetaOld:   testObject2.GetObjectMeta(),
		ObjectOld: testObject2,
		MetaNew:   updated2.GetObjectMeta(),
		ObjectNew: updated2,
	}, q)
	assertEmptyQueue()

	// updates on the first object should still work
	d.Update(event.UpdateEvent{
		MetaOld:   testObject1.GetObjectMeta(),
		ObjectOld: testObject1,
		MetaNew:   updated1.GetObjectMeta(),
		ObjectNew: updated1,
	}, q)
	assertReconcileReq(watching)

	// setup an owner watch where owner is testObject1
	require.NoError(t, d.AddHandler(&OwnerWatch{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			OwnerType:    testObject1,
			IsController: true,
		},
	}))

	// let's make object 1 the owner of object 2
	require.NoError(t, controllerutil.SetControllerReference(testObject1, testObject2, scheme.Scheme))
	// an update on object 2 should enqueue a request for object 1 (the owner)
	d.Update(event.UpdateEvent{
		MetaOld:   testObject2.GetObjectMeta(),
		ObjectOld: testObject2,
		MetaNew:   updated2.GetObjectMeta(),
		ObjectNew: updated2,
	}, q)
	assertReconcileReq(nsn1)
	// same for deletes
	d.Delete(event.DeleteEvent{
		Object: testObject2,
		Meta:   testObject2.GetObjectMeta(),
	}, q)
	assertReconcileReq(nsn1)

	// named watch on object 1 should still work
	d.Create(event.CreateEvent{
		Object: testObject1,
		Meta:   testObject1.GetObjectMeta(),
	}, q)
	assertReconcileReq(watching)

	// it's possible to have both an owner watch and a named watch triggered
	// for a single event
	// add a named watch on object 2
	require.NoError(t, d.AddHandler(NamedWatch{
		Watched: nsn2,
		Watcher: watching,
		Name:    "test-watch-2",
	}))
	d.Create(event.CreateEvent{
		Object: testObject2,
		Meta:   testObject2.GetObjectMeta(),
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
