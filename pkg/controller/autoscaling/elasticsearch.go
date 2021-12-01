// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaling

import (
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/predicates"
)

const (
	controllerName = "elasticsearch-autoscaling"
)

// Add creates a new Elasticsearch autoscaling controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, p operator.Parameters) error {
	r := elasticsearch.NewReconciler(mgr, p)
	c, err := common.NewController(mgr, controllerName, r, p)
	if err != nil {
		return err
	}
	// Watch for changes on Elasticsearch clusters.
	return c.Watch(&source.Kind{Type: &esv1.Elasticsearch{}}, &handler.EnqueueRequestForObject{}, predicates.ManagedNamespacePredicate)
}
