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

var _ controller.Controller = &namespaceAwareController{}

// namespaceAwareController implements the controller.Controller interface and automatically include a predicate to filter events
// which are not in a managed namespace.
type namespaceAwareController struct {
	controller.Controller
	namespacePredicate predicate.Predicate
}

func newNamespaceAwareWatchersController(c controller.Controller, managedNamespaces []string, operatorNamespace string) controller.Controller {
	watchedNamespaces := managedNamespaces
	if !slices.Contains(managedNamespaces, operatorNamespace) {
		watchedNamespaces = append(watchedNamespaces, operatorNamespace)
	}
	return &namespaceAwareController{
		Controller:         c,
		namespacePredicate: predicates.NewManagedNamespacesPredicate(managedNamespaces),
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
