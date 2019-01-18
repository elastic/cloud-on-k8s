package watches

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sync"
)

type ToReconcileRequestTransformer interface {
	Key() string
	ToReconcileRequest(object metav1.Object) []reconcile.Request
}


type DynamicEnqueueRequests struct {
	mutex sync.RWMutex
	transformers map[string]ToReconcileRequestTransformer
}

func (d *DynamicEnqueueRequests) AddWatch(xform ToReconcileRequestTransformer) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.transformers[xform.Key()] = xform
}

func (d *DynamicEnqueueRequests) RemoveWatch(xform ToReconcileRequestTransformer) {
	d.RemoveWatchForKey(xform.Key())
}

func (d *DynamicEnqueueRequests) RemoveWatchForKey(key string) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	delete(d.transformers, key)
}

// DynamicEnqueueRequests implements EventHandler
var _ handler.EventHandler = &DynamicEnqueueRequests{}

func (d *DynamicEnqueueRequests) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _,v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			q.Add(req)
		}
	}
}

func (d *DynamicEnqueueRequests) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _,v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.MetaOld) {
			q.Add(req)
		}
		for _, req := range v.ToReconcileRequest(evt.MetaNew) {
			q.Add(req)
		}

	}
}

func (d *DynamicEnqueueRequests) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _,v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			q.Add(req)
		}
	}
}

func (d *DynamicEnqueueRequests) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	for _,v := range d.transformers {
		for _, req := range v.ToReconcileRequest(evt.Meta) {
			q.Add(req)
		}
	}
}