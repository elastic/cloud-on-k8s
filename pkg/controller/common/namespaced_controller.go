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

// NewNamespacedController creates a controller that is aware of the operator's dynamic namespace
// selector (--namespace-selector). It is meant to be used instead of NewController by controllers
// whose reconciliations must be restricted to the set of namespaces currently matching the selector.
//
// When the selector is disabled, it behaves exactly like NewController: the given reconciler is
// registered as-is with no extra overhead.
//
// When the selector is enabled, the reconciler is wrapped so that every reconciliation is:
//   - gated on an Enterprise license, since the dynamic namespace selector is an enterprise feature
//     (reconciliations are skipped and retried later while enterprise features are disabled);
//   - skipped when the request's namespace no longer matches the selector, in which case
//     OnNamespaceOutOfScope is invoked to let the reconciler drop any state it holds for
//     the resource (e.g. dynamic watches, in-memory associations).
//
// In addition, the controller watches namespace label changes that flip a namespace in or out of
// the selector's scope. On such a change, requestsForNamespace is called to enumerate the
// reconcile requests to enqueue for that namespace, so resources are picked up (or cleaned up)
// without waiting for another event on them.
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

// NamespacedReconciler is a reconcile.Reconciler that can additionally be notified when a resource
// falls out of the operator's namespace scope, to release any state it holds for that resource.
type NamespacedReconciler interface {
	reconcile.Reconciler
	// OnNamespaceOutOfScope is called instead of Reconcile when the resource's namespace no longer
	// matches the namespace selector. Implementations should clean up any internal state associated
	// with the resource (dynamic watches, caches, etc.) but must not touch the resource itself.
	OnNamespaceOutOfScope(resource types.NamespacedName)
}

// namespacedReconcilerWrapper enforces the enterprise license and namespace selector checks
// before delegating to the inner reconciler. See NewNamespacedController.
type namespacedReconcilerWrapper struct {
	inner          NamespacedReconciler
	parameters     operator.Parameters
	licenseChecker license.Checker
	recorder       toolsevents.EventRecorder
}

func (r *namespacedReconcilerWrapper) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := ulog.FromContext(ctx)

	matches, err := r.parameters.NamespaceMatcher.NamespaceNameMatches(ctx, request.Namespace)
	if err != nil {
		// Treat a failed match check the same as a non-matching namespace: skip
		// reconciliation and let the inner reconciler clean up any state it holds
		// for this resource, rather than requeuing on an error that a namespace
		// selector check is unlikely to recover from on its own.
		log.Error(err, "Failed to check namespace selector match; treating as out of scope", "namespace", request.Namespace, "name", request.Name)
		r.inner.OnNamespaceOutOfScope(request.NamespacedName)
		return reconcile.Result{}, nil
	}
	if !matches {
		// The namespace no longer matches the selector: skip reconciliation and let
		// the inner reconciler clean up any state it holds for this resource.
		log.V(1).Info("Skipping reconciliation: namespace out of scope", "namespace", request.Namespace, "name", request.Name)
		r.inner.OnNamespaceOutOfScope(request.NamespacedName)
		return reconcile.Result{}, nil
	}

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

	return r.inner.Reconcile(ctx, request)
}
