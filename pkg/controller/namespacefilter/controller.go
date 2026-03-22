// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package namespacefilter

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const controllerName = "namespace-filter-controller"

func Add(mgr manager.Manager, params operator.Parameters) error {
	if params.NamespaceFilter == nil {
		return nil
	}

	r := &ReconcileNamespaceFilter{
		Client:          mgr.GetClient(),
		recorder:        mgr.GetEventRecorderFor(controllerName),
		Parameters:      params,
		namespaceFilter: params.NamespaceFilter,
	}

	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}

	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}, &handler.TypedEnqueueRequestForObject[*corev1.Namespace]{}))
}

type ReconcileNamespaceFilter struct {
	k8s.Client
	recorder record.EventRecorder
	operator.Parameters
	namespaceFilter *operator.NamespaceFilter
	iteration       uint64
}

func (r *ReconcileNamespaceFilter) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "namespace", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)
	log := ulog.FromContext(ctx)

	var namespace corev1.Namespace
	err := r.Get(ctx, request.NamespacedName, &namespace)
	if apierrors.IsNotFound(err) {
		wasManaged := r.namespaceFilter.ShouldManage(request.Name)
		r.namespaceFilter.OnNamespaceDelete(request.Name)
		if wasManaged {
			log.V(1).Info("Namespace removed from managed set (namespace deleted)", "namespace", request.Name)
		}
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	wasManaged := r.namespaceFilter.ShouldManage(namespace.Name)
	r.namespaceFilter.OnNamespaceUpsert(namespace)
	isManaged := r.namespaceFilter.ShouldManage(namespace.Name)

	if !wasManaged && isManaged {
		log.V(1).Info("Namespace included in managed set by selector", "namespace", namespace.Name, "labels", namespace.Labels)
	} else if wasManaged && !isManaged {
		log.V(1).Info("Namespace excluded from managed set by selector", "namespace", namespace.Name, "labels", namespace.Labels)
	}

	return reconcile.Result{}, nil
}
