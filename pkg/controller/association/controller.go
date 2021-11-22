// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
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
	return addWatches(c, r, associationInfo.Predicates)
}

func addWatches(c controller.Controller, r *Reconciler, predicates []predicate.Predicate) error {
	// Watch the associated resource (e.g. Kibana for a Kibana -> Elasticsearch association)
	if err := c.Watch(&source.Kind{Type: r.AssociatedObjTemplate()}, &handler.EnqueueRequestForObject{}, predicates...); err != nil {
		return err
	}

	// Watch Secrets owned by the associated resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    r.AssociatedObjTemplate(),
		IsController: true,
	}, predicates...); err != nil {
		return err
	}

	// Dynamically watch the referenced resources (e.g. Elasticsearch B for a Kibana A -> Elasticsearch B association)
	if err := c.Watch(&source.Kind{Type: r.ReferencedObjTemplate()}, r.watches.ReferencedResources, predicates...); err != nil {
		return err
	}

	// Dynamically watch Secrets (CA Secret of the referenced resource and ES user secret)
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.watches.Secrets, predicates...); err != nil {
		return err
	}

	// Dynamically watch Service objects for custom services setup by the user
	return c.Watch(&source.Kind{Type: &corev1.Service{}}, r.watches.Services, predicates...)
}
