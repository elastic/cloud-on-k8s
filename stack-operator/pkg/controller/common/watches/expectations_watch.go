package watches

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/label"
	"k8s.io/client-go/util/workqueue"
	k8sctl "k8s.io/kubernetes/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

type ExpectationsWatch struct {
	handlerKey   string
	expectations *k8sctl.UIDTrackingControllerExpectations
}

func NewExpectationsWatch(handlerKey string, expectations *k8sctl.UIDTrackingControllerExpectations) *ExpectationsWatch {
	return &ExpectationsWatch{
		handlerKey:   handlerKey,
		expectations: expectations,
	}
}

func (p *ExpectationsWatch) Key() string {
	return p.handlerKey
}

func (p *ExpectationsWatch) EventHandler() handler.EventHandler {
	return p
}

// Create returns true if the Create event should be processed
func (p *ExpectationsWatch) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	clusterName, exists := evt.Meta.GetLabels()[label.ClusterNameLabelName]
	if exists {
		key := evt.Meta.GetNamespace() + "/" + clusterName
		p.expectations.CreationObserved(key)
		log.V(4).Info("Marking expectations creation observed", "key", key)
	}
}

// Update returns true if the Update event should be processed
func (p *ExpectationsWatch) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

// Delete returns true if the Delete event should be processed
func (p *ExpectationsWatch) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	clusterName, exists := evt.Meta.GetLabels()[label.ClusterNameLabelName]
	if exists {
		key := evt.Meta.GetNamespace() + "/" + clusterName
		p.expectations.DeletionObserved(key, evt.Meta.GetName())
		log.V(4).Info("Marking expectations deletion observed", "key", key, "resource", evt.Meta.GetName())
	}
}

// Generic returns true if the Generic event should be processed
func (p *ExpectationsWatch) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

var _ HandlerRegistration = &ExpectationsWatch{}
