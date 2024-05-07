// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoscaling

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/autoscaling/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
)

// Add creates a new Elasticsearch autoscaling controllers, and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controllers and Start them when the Manager is Started.
func Add(mgr manager.Manager, p operator.Parameters) error {
	reconciler := elasticsearch.NewReconciler(mgr, p)

	// The CRD based controller watches for changes on both the ElasticsearchAutoscaler CRD, and on the Elasticsearch resources to make sure the
	// NodeSets resources are reconciled with the required resources.
	controller, err := common.NewController(mgr, elasticsearch.ControllerName, reconciler, p)
	if err != nil {
		return err
	}
	if err := controller.Watch(source.Kind(mgr.GetCache(), &v1alpha1.ElasticsearchAutoscaler{}, &handler.TypedEnqueueRequestForObject[*v1alpha1.ElasticsearchAutoscaler]{})); err != nil {
		return err
	}
	return controller.Watch(source.Kind[client.Object](mgr.GetCache(), &esv1.Elasticsearch{}, reconciler.Watches.ReferencedResources))
}
