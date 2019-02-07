// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// ExpectationsResourceRetriever is a function that allows retrieving, from a given resource,
// the associated resource that holds expectations resources.
// For instance, from a given pod, we might want to retrieve the ElasticsearchCluster associated
// to it (see `label.ClusterFromResourceLabels`).
type ExpectationsResourceRetriever func(metaObject metav1.Object) (types.NamespacedName, bool)

// ExpectationsWatch is an event handler for watches that markes resources creations and deletions
// as observed for the given reconciler expectations.
type ExpectationsWatch struct {
	handlerKey        string
	expectations      *reconciler.Expectations
	resourceRetriever ExpectationsResourceRetriever
}

// Make sure our ExpectationsWatch implements HandlerRegistration.
var _ HandlerRegistration = &ExpectationsWatch{}

// NewExpectationsWatch creates an ExpectationsWatch from the given arguments.
func NewExpectationsWatch(handlerKey string, expectations *reconciler.Expectations, resourceRetriever ExpectationsResourceRetriever) *ExpectationsWatch {
	return &ExpectationsWatch{
		handlerKey:        handlerKey,
		expectations:      expectations,
		resourceRetriever: resourceRetriever,
	}
}

// Key returns the key associated to this handler.
func (p *ExpectationsWatch) Key() string {
	return p.handlerKey
}

// EventHandler returns the ExpectationsWatch as an handler.EventHandler.
func (p *ExpectationsWatch) EventHandler() handler.EventHandler {
	return p
}

// Create marks a resource creation as observed in the expectations.
func (p *ExpectationsWatch) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	resource, exists := p.resourceRetriever(evt.Meta)
	if exists {
		p.expectations.CreationObserved(resource)
		log.V(4).Info("Marking creation observed in expectations", "resource", resource)
	}
}

// Delete marks a resource deletion as observed in the expectations.
func (p *ExpectationsWatch) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	resource, exists := p.resourceRetriever(evt.Meta)
	if exists {
		p.expectations.DeletionObserved(resource)
		log.V(4).Info("Marking deletion observed in expectations", "resource", resource)
	}
}

// Update is a no-op operation in this context.
func (p *ExpectationsWatch) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {}

// Generic is a no-op operation in this context.
func (p *ExpectationsWatch) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {}
