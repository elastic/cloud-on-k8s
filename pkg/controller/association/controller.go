// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

// AddAssociationController sets up and starts an association controller for the given associationInfo.
func AddAssociationController(
	mgr manager.Manager,
	accessReviewer rbac.AccessReviewer,
	params operator.Parameters,
	associationInfo AssociationInfo,
) error {
	// Derive the referenced resource Kind from the scheme at setup time,
	// making the relationship with ReferencedObjTemplate explicit and unbreakable.
	referencedResourceKind, err := referencedObjKind(mgr, associationInfo.ReferencedObjTemplate())
	if err != nil {
		return err
	}

	controllerName := associationInfo.AssociationName + "-association-controller"
	r := &Reconciler{
		AssociationInfo:        associationInfo,
		Client:                 mgr.GetClient(),
		accessReviewer:         accessReviewer,
		watches:                watches.NewDynamicWatches(),
		recorder:               mgr.GetEventRecorder(controllerName),
		Parameters:             params,
		referencedResourceKind: referencedResourceKind,
	}
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

func addWatches(mgr manager.Manager, c controller.Controller, r *Reconciler) error {
	m := r.NamespaceMatcher
	// Watch the associated resource (e.g. Kibana for a Kibana -> Elasticsearch association)
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), r.AssociatedObjTemplate(), &handler.TypedEnqueueRequestForObject[commonv1.Associated]{})); err != nil {
		return err
	}

	// Watch Secrets owned by the associated resource
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		r.AssociatedObjTemplate(), handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Dynamically watch the referenced resources (e.g. Elasticsearch B for a Kibana A -> Elasticsearch B association)
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), r.ReferencedObjTemplate(), r.watches.ReferencedResources)); err != nil {
		return err
	}

	// Dynamically watch Secrets (CA Secret of the referenced resource, ES user secret or custom referenced object secret)
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Secret{}, r.watches.Secrets)); err != nil {
		return err
	}

	// Dynamically watch Service objects for custom services setup by the user
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Service{}, r.watches.Services)); err != nil {
		return err
	}
	return watches.WatchNamespaceFlips(c, mgr.GetCache(), r.NamespaceMatcher, r.AssociatedObjListTemplate)
}

// referencedObjKind derives the Kind of the referenced resource from the manager's scheme.
func referencedObjKind(mgr manager.Manager, obj client.Object) (string, error) {
	gvks, _, err := mgr.GetScheme().ObjectKinds(obj)
	if err != nil {
		return "", fmt.Errorf("failed to derive Kind for %T: %w", obj, err)
	}
	if len(gvks) == 0 || gvks[0].Kind == "" {
		return "", fmt.Errorf("no GVK found for %T", obj)
	}
	return gvks[0].Kind, nil
}
