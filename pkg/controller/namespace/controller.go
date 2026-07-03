// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package namespace contains the match-state controller used in dynamic
// namespaceSelector mode. It watches Namespace objects and, whenever a
// namespace's match-state against the configured selector changes, broadcasts
// the Namespace to all per-kind controllers via a NamespaceMatcher.
// Each per-kind controller subscribes to the notifier and uses
// handler.EnqueueRequestsFromMapFunc to list its own CRs in the namespace and
// enqueue reconcile requests — no API writes are needed.
package namespace

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const controllerName = "namespace-controller"

// Add registers the namespace match-state controller with the manager. It is
// only meaningful when the matcher is enabled (dynamic mode); the caller
// should skip registration otherwise.
func Add(mgr manager.Manager, params operator.Parameters) error {
	if !params.NamespaceMatcher.SelectorEnabled() {
		return nil
	}
	r := &reconciler{
		cache:             mgr.GetCache(),
		nsMatchNotifier:   params.NamespaceMatcher,
		licenseChecker:    license.NewLicenseChecker(mgr.GetClient(), params.OperatorNamespace),
		recorder:          mgr.GetEventRecorder(controllerName),
		operatorNamespace: params.OperatorNamespace,
	}

	seedLog := mgr.GetLogger().WithName(controllerName).WithName("seeder")
	if err := mgr.Add(&namespaceSeedRunnable{log: seedLog, client: mgr.GetCache(), namespaceMatcher: params.NamespaceMatcher}); err != nil {
		return fmt.Errorf("registering namespace init runnable: %w", err)
	}
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}, &handler.TypedEnqueueRequestForObject[*corev1.Namespace]{}))
}

type reconciler struct {
	cache             cache.Cache
	nsMatchNotifier   *nsmatch.NamespaceMatcher
	licenseChecker    license.Checker
	recorder          toolsevents.EventRecorder
	operatorNamespace string
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := ulog.FromContext(ctx).WithValues("namespace", request.Name)

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !enabled {
		const msg = "Dynamic namespace selector is an enterprise feature. Enterprise features are disabled"
		log.V(1).Info(msg)
		license.EmitEnterpriseFeatureEvent(r.recorder, r.operatorNamespace, msg)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	return r.doReconcile(ctx, log, request)
}

func (r *reconciler) doReconcile(ctx context.Context, log logr.Logger, request reconcile.Request) (reconcile.Result, error) {
	var ns corev1.Namespace
	if err := r.cache.Get(ctx, types.NamespacedName{Name: request.Name}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("namespace deleted")
			// Namespace was deleted: mark it as non-matching without broadcasting. All resources
			// that lived in it are being deleted or cleaned up by their own controllers, so there
			// is nothing for the namespace-selector logic to react to.
			r.nsMatchNotifier.ForgetNamespace(request.Name)
			return reconcile.Result{}, nil
		}
		log.Error(err, "error while fetching namespace")
		return reconcile.Result{}, err
	}

	stateChanged, isMatching, err := r.nsMatchNotifier.ObserveAndBroadcast(ctx, &ns)
	if err != nil {
		log.Error(err, "error while broadcasting namespace match change", "namespace", ns.Name)
	}
	if stateChanged {
		log.Info("namespace match-state changed", "matches", isMatching)
	}

	return reconcile.Result{}, nil
}

var _ reconcile.Reconciler = (*reconciler)(nil)

// namespaceSeedRunnable seeds the NamespaceMatcher with the current match state
// of all existing namespaces. Registered via mgr.Add, it starts after the
// manager's cache has synced, so it reads from the cache rather than the API
// server. The manager provides no ordering between this runnable and the
// controllers: they all start concurrently, so watch predicates and reconcile
// loops may briefly observe an unseeded matcher (Matches returning false).
// Correctness does not depend on that ordering: ObserveAndBroadcast broadcasts
// every false->true transition it seeds, and each per-kind controller's
// namespace-flip watch reacts by re-enqueueing all CRs in the broadcast
// namespace, backfilling any events the predicates dropped during the
// cold-start window.
type namespaceSeedRunnable struct {
	log              logr.Logger
	client           client.Reader
	namespaceMatcher *nsmatch.NamespaceMatcher
}

func (r *namespaceSeedRunnable) Start(ctx context.Context) error {
	var nsList corev1.NamespaceList
	if err := r.client.List(ctx, &nsList); err != nil {
		return err
	}

	// Run in a goroutine so that ObserveAndBroadcast can block waiting for
	// subscribers to start consuming without holding up the Runnable that
	// called us.
	go func() {
		for _, ns := range nsList.Items {
			if ctx.Err() != nil {
				return
			}
			if _, _, err := r.namespaceMatcher.ObserveAndBroadcast(ctx, &ns); err != nil {
				r.log.Error(err, "failed to seed namespace match state", "namespace", ns.Name)
			}
		}
	}()

	return nil
}

func (r *namespaceSeedRunnable) NeedLeaderElection() bool { return true }
