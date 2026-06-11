// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package namespace contains the flip-state controller used in dynamic
// namespaceSelector mode. It watches Namespace objects and, whenever a
// namespace's match-state against the configured selector changes, broadcasts
// the Namespace to all per-kind controllers via a NamespaceFlipNotifier.
// Each per-kind controller subscribes to the notifier and uses
// handler.EnqueueRequestsFromMapFunc to list its own CRs in the namespace and
// enqueue reconcile requests — no API writes are needed.
package namespace

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const controllerName = "namespace-controller"

// Add registers the namespace flip-state controller with the manager. It is
// only meaningful when the matcher is enabled (dynamic mode); the caller
// should skip registration otherwise.
func Add(mgr manager.Manager, params operator.Parameters) error {
	if !params.NamespaceMatchNotifier.SelectorEnabled() {
		return nil
	}
	r := &reconciler{
		client:          mgr.GetClient(),
		nsMatchNotifier: params.NamespaceMatchNotifier,
		states:          nsmatch.NewNamespaceStates(),
	}
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}, &handler.TypedEnqueueRequestForObject[*corev1.Namespace]{}))
}

type reconciler struct {
	client          client.Client
	nsMatchNotifier *nsmatch.MatchNotifier

	states *nsmatch.NamespaceStates
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := ulog.FromContext(ctx).WithValues("namespace", request.Name)

	var ns corev1.Namespace
	if err := r.client.Get(ctx, types.NamespacedName{Name: request.Name}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			r.states.Forget(request.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	isMatching := r.nsMatchNotifier.MatchesLabels(ns.Labels)
	wasMatching, known := r.states.Swap(request.Name, isMatching)
	if known && wasMatching == isMatching {
		// no state change, no reconciliation is required.
		return reconcile.Result{}, nil
	}

	log.Info("namespace match-state changed", "matches", isMatching, "previously_known", known)
	r.nsMatchNotifier.Broadcast(&ns)
	return reconcile.Result{}, nil
}

var _ reconcile.Reconciler = (*reconciler)(nil)
