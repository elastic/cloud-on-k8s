// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package autoscaling

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/autoscaling/elasticsearch"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	if err := c.Watch(
		&source.Kind{Type: &esv1.Elasticsearch{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}
	return nil
}
