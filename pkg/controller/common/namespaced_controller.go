// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

func NewNamespacedController(mgr manager.Manager, name string, r NamespacedReconciler, p operator.Parameters, requestsForNamespace func(context.Context, *corev1.Namespace) []reconcile.Request) (controller.Controller, error) {
	if !p.NamespaceMatcher.SelectorEnabled() {
		return NewController(mgr, name, r, p)
	}

	lc := license.NewLicenseChecker(mgr.GetClient(), p.OperatorNamespace)

	wrapped := &namespacedReconcilerWrapper{inner: r, parameters: p, licenseChecker: lc, recorder: mgr.GetEventRecorder(name)}
	c, err := NewController(mgr, name, wrapped, p)
	if err != nil {
		return nil, err
	}

	// watch for namespace scope changes
	err = watches.WatchNamespaceScopeChange(c, mgr.GetCache(), p.NamespaceMatcher, requestsForNamespace)

	return c, err
}

type NamespacedReconciler interface {
	reconcile.Reconciler
	OnNamespaceOutOfScope(resource types.NamespacedName)
}

type namespacedReconcilerWrapper struct {
	inner          NamespacedReconciler
	parameters     operator.Parameters
	licenseChecker license.Checker
	recorder       toolsevents.EventRecorder
}

func (r *namespacedReconcilerWrapper) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := ulog.FromContext(ctx)

	// the dynamic namespace selector is an enterprise feature: this wrapper is only installed
	// when the selector is enabled, so gate every reconciliation on the license.
	ok, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		log.Error(err, "error while checking license")
		return reconcile.Result{}, err
	}
	if !ok {
		const msg = "Dynamic namespace selector is an enterprise feature. Enterprise features are disabled"
		log.V(1).Info(msg)
		license.EmitEnterpriseFeatureEvent(r.recorder, r.parameters.OperatorNamespace, msg)
		return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if !r.parameters.NamespaceMatcher.NamespaceNameMatches(ctx, request.Namespace) {
		// The namespace no longer matches the selector: skip reconciliation and let
		// the inner reconciler clean up any state it holds for this resource.
		log.V(2).Info("Skipping reconciliation: namespace out of scope", "namespace", request.Namespace, "name", request.Name)
		r.inner.OnNamespaceOutOfScope(request.NamespacedName)
		return reconcile.Result{}, nil
	}

	return r.inner.Reconcile(ctx, request)
}
