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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
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

// Add registers the namespace flip-state controller with the manager. It is
// only meaningful when the matcher is enabled (dynamic mode); the caller
// should skip registration otherwise.
func Add(mgr manager.Manager, params operator.Parameters) error {
	if !params.NamespaceMatchNotifier.SelectorEnabled() {
		return nil
	}
	r := &reconciler{
		client:            mgr.GetClient(),
		nsMatchNotifier:   params.NamespaceMatchNotifier,
		licenseChecker:    license.NewLicenseChecker(mgr.GetClient(), params.OperatorNamespace),
		recorder:          mgr.GetEventRecorder(controllerName),
		operatorNamespace: params.OperatorNamespace,
	}

	if err := mgr.Add(&nsInitRunnable{client: mgr.GetClient(), notifier: params.NamespaceMatchNotifier}); err != nil {
		return fmt.Errorf("registering namespace init runnable: %w", err)
	}
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}, &handler.TypedEnqueueRequestForObject[*corev1.Namespace]{}))
}

type reconciler struct {
	client            client.Client
	nsMatchNotifier   *nsmatch.NamespaceFlipNotifier
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
	if err := r.client.Get(ctx, types.NamespacedName{Name: request.Name}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			r.nsMatchNotifier.ForgetNamespace(request.Name) // namespace got deleted.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if stateChanged, isMatching := r.nsMatchNotifier.ObserveAndBroadcast(&ns); stateChanged {
		log.Info("namespace match-state changed", "matches", isMatching)
	}

	return reconcile.Result{}, nil
}

var _ reconcile.Reconciler = (*reconciler)(nil)

// nsInitRunnable seeds the NamespaceFlipNotifier with the current match state
// of all existing namespaces. It is registered with the manager so that it runs
// after the cache is synced, reading from the cache rather than the API server.
type nsInitRunnable struct {
	client   client.Reader
	notifier *nsmatch.NamespaceFlipNotifier
}

func (r *nsInitRunnable) Start(ctx context.Context) error {
	var nsList corev1.NamespaceList
	if err := r.client.List(ctx, &nsList); err != nil {
		return err
	}

	go func() {
		for _, ns := range nsList.Items {
			_, _ = r.notifier.ObserveAndBroadcast(&ns)
		}
	}()

	return nil
}

func (r *nsInitRunnable) NeedLeaderElection() bool { return true }
