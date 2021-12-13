// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"strconv"
	"sync/atomic"

	"go.elastic.co/apm"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/predicates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

// NewController creates a new controller with the given name, reconciler and parameters and registers it with the manager.
func NewController(mgr manager.Manager, name string, r reconcile.Reconciler, p operator.Parameters) (controller.Controller, error) {
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: p.MaxConcurrentReconciles})
	if err != nil {
		return nil, err
	}
	return newNamespaceAwareWatchersController(c, p.ManagedNamespaces, p.OperatorNamespace), nil
}

// NewControllerWithOptions creates a new controller with the given name, reconciler, parameters, options and registers it with the manager.
func NewControllerWithOptions(mgr manager.Manager, name string, p operator.Parameters, options controller.Options) (controller.Controller, error) {
	c, err := controller.New(name, mgr, options)
	if err != nil {
		return nil, err
	}
	return newNamespaceAwareWatchersController(c, p.ManagedNamespaces, p.OperatorNamespace), nil
}

var _ controller.Controller = &namespaceAwareController{}

// namespaceAwareController implements the controller.Controller interface and automatically include a predicate to filter events
// which are not in a managed namespace.
type namespaceAwareController struct {
	controller.Controller
	namespacePredicate predicate.Predicate
}

// newNamespaceAwareWatchersController creates a new namespaceAwareController, ensuring that a predicate exists to ignore any
// namespaced events outside of managed namespaces, and the operator namespace.
func newNamespaceAwareWatchersController(c controller.Controller, managedNamespaces []string, operatorNamespace string) controller.Controller {
	watchedNamespaces := managedNamespaces
	// if the length of watchedNamespaces is 0, then we're watching all namespaces, and shouldn't append anything to the slice, as
	// it will just cause issues wth the managed namespaces predicate.
	if len(watchedNamespaces) > 0 && !slices.Contains(watchedNamespaces, operatorNamespace) {
		watchedNamespaces = append(watchedNamespaces, operatorNamespace)
	}
	return &namespaceAwareController{
		Controller:         c,
		namespacePredicate: predicates.NewManagedNamespacesPredicate(watchedNamespaces),
	}
}

// Watch implements controller.Controller interface, and calls the underlying controller's
// watch method, ensuring that the namespace predicate exists.
func (n *namespaceAwareController) Watch(src source.Source, eventhandler handler.EventHandler, predicates ...predicate.Predicate) error {
	return n.Controller.Watch(src, eventhandler, append(predicates, n.namespacePredicate)...)
}

// NewReconciliationContext increments iteration, creates an apm transaction and initiates the logger. Returns context
// with apm transaction metadata and configured logger.
func NewReconciliationContext(
	ctx context.Context,
	iteration *uint64,
	tracer *apm.Tracer,
	controllerName, nameField string,
	request reconcile.Request,
) context.Context {
	it := atomic.AddUint64(iteration, 1)
	itString := strconv.FormatUint(it, 10)
	newCtx := tracing.NewContextTransaction(
		ctx,
		tracer,
		controllerName,
		request.String(),
		map[string]string{"iteration": itString})
	return logconf.InitInContext(
		newCtx,
		controllerName,
		"iteration", itString,
		"namespace", request.Namespace,
		nameField, request.Name)
}
