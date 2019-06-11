/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewWatch(fn handler.Mapper) watches.HandlerRegistration {
	return &watch{
		EnqueueRequestsFromMapFunc: handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(
				func(object handler.MapObject) []reconcile.Request {
					labels := object.Meta.GetLabels()
					if labels[common.TypeLabelName] == Type && labels[LicenseLabelType] == string(LicenseTypeEnterprise) {
						return fn.Map(object)
					}
					return nil
				}),
		},
	}
}

type watch struct {
	handler.EnqueueRequestsFromMapFunc
}

func (w *watch) EventHandler() handler.EventHandler {
	return w
}

func (w *watch) Key() string {
	return "enterprise-license-watch"
}

var _ watches.HandlerRegistration = &watch{}
