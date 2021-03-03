// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log = ulog.Log.WithName("association")
)

// AddAssociationController sets up and starts an association controller for the given associationInfo.
func AddAssociationController(
	mgr manager.Manager,
	accessReviewer rbac.AccessReviewer,
	params operator.Parameters,
	associationInfo AssociationInfo,
) error {
	controllerName := associationInfo.AssociationName + "-association-controller"
	r := &Reconciler{
		AssociationInfo: associationInfo,
		Client:          mgr.GetClient(),
		accessReviewer:  accessReviewer,
		watches:         watches.NewDynamicWatches(),
		recorder:        mgr.GetEventRecorderFor(controllerName),
		Parameters:      params,
		// override the default logger to be specialized with the association name
		logger: log.WithName(controllerName),
	}
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

func addWatches(c controller.Controller, r *Reconciler) error {
	// Watch the associated resources
	if err := c.Watch(&source.Kind{Type: r.AssociatedObjTemplate()}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch Elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &esv1.Elasticsearch{}}, r.watches.ElasticsearchClusters); err != nil {
		return err
	}

	// Dynamically watch Elasticsearch public CA secrets for referenced ES clusters
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.watches.Secrets); err != nil {
		return err
	}

	// Watch Secrets owned by the associated resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    r.AssociatedObjTemplate(),
		IsController: true,
	}); err != nil {
		return err
	}

	return nil
}
