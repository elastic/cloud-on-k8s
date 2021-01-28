// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewWatch(fn handler.MapFunc) watches.HandlerRegistration {
	return &watch{
		evtHandler: handler.EnqueueRequestsFromMapFunc(func(object client.Object) []reconcile.Request {
			labels := object.GetLabels()
			if labels[common.TypeLabelName] == Type && labels[LicenseLabelType] == string(LicenseTypeEnterprise) {
				return fn(object)
			}
			return nil
		}),
	}
}

type watch struct {
	evtHandler handler.EventHandler
}

func (w *watch) EventHandler() handler.EventHandler {
	return w.evtHandler
}

func (w *watch) Key() string {
	return "enterprise-license-watch"
}

var _ watches.HandlerRegistration = &watch{}
