// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func reconcileRequestsForAllClusters(c k8s.Client, log logr.Logger) ([]reconcile.Request, error) {
	var clusters esv1.ElasticsearchList
	// list all clusters
	err := c.List(context.Background(), &clusters)
	if err != nil {
		return nil, err
	}

	// create a reconcile request for each cluster
	requests := make([]reconcile.Request, len(clusters.Items))
	for i, cl := range clusters.Items {
		log.V(1).Info("Generating license reconcile event for ES cluster", "name", cl.Name, "namespace", cl.Namespace)
		requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: cl.Namespace,
			Name:      cl.Name,
		}}
	}
	return requests, nil
}
