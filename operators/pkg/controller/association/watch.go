// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func addWatches(c controller.Controller, r *ReconcileAssociation) error {
	// Watch for changes to Kibana resources
	if err := c.Watch(&source.Kind{Type: &kbtype.Kibana{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Dynamically watch related Elasticsearch resources (not all ES resources)
	if err := c.Watch(&source.Kind{Type: &estype.Elasticsearch{}}, r.watches.ElasticsearchClusters); err != nil {
		return err
	}

	// Watch Users owned by a Kibana resource
	if err := c.Watch(&source.Kind{Type: &estype.User{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &kbtype.Kibana{},
		IsController: true,
	}); err != nil {
		return err
	}

	// Watch Secrets owned by a Kibana resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &kbtype.Kibana{},
		IsController: true,
	}); err != nil {
		return err
	}

	return nil
}

// elasticsearchWatchName returns the name of the watch setup on an Elasticsearch cluster
// for a given Kibana resource.
func elasticsearchWatchName(kibanaKey types.NamespacedName) string {
	return kibanaKey.Namespace + "-" + kibanaKey.Name + "-es-watch"
}

// watchFinalizer ensure that we remove watches for Elasticsearch clusters that we are no longer interested in
// because not referenced by any Kibana resource.
func watchFinalizer(kibanaKey types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches.finalizers.associations.k8s.elastic.co",
		Execute: func() error {
			w.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(kibanaKey))
			return nil
		},
	}
}
