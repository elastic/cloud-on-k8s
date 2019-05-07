// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GenericResources are resources that all clusters have.
type GenericResources struct {
	// ExternalService is the user-facing service
	ExternalService corev1.Service
	// DiscoveryService is the service used by ES for discovery purposes
	DiscoveryService corev1.Service
}

// reconcileGenericResources reconciles the expected generic resources of a cluster.
func reconcileGenericResources(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
) (*GenericResources, error) {
	// TODO: these reconciles do not necessarily use the services as in-out params.
	// TODO: consider removing the "res" bits of the ReconcileService signature?

	discoveryService := services.NewDiscoveryService(es)
	_, err := common.ReconcileService(c, scheme, discoveryService, &es)
	if err != nil {
		return nil, err
	}

	externalService := services.NewExternalService(es)
	_, err = common.ReconcileService(c, scheme, externalService, &es)
	if err != nil {
		return nil, err
	}

	return &GenericResources{DiscoveryService: *discoveryService, ExternalService: *externalService}, nil
}
