// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewController creates a new controller with the given name, reconciler and parameters and registers it with the manager.
func NewController(mgr manager.Manager, name string, r reconcile.Reconciler, p operator.Parameters) (controller.Controller, error) {
	return controller.New(name, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: p.MaxConcurrentReconciles})
}
