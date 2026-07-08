// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package association

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
	c, err := common.NewNamespacedController(mgr, controllerName, r, params, namespaceFlipRequests(mgr.GetCache(), r))
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
	return c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Service{}, r.watches.Services))
}

// namespaceFlipRequests returns a mapper translating a namespace match-state change into
// reconcile requests for the associated resources affected by it: the ones living in the
// flipped namespace, and the ones living elsewhere whose association references a resource
// in the flipped namespace (e.g. a Kibana in an in-scope namespace referencing an
// Elasticsearch in the namespace that just went out of scope). The latter cannot rely on the
// dynamic referenced-resource watch: a namespace label change produces no event on the
// referenced object, and the NamespacedKind predicate drops events from de-scoped namespaces.
//
// Known limitation: transitive references (e.g. Agent -> Fleet Server -> Elasticsearch, each
// in a different namespace) are matched on the direct reference only; a flip of the transitive
// Elasticsearch namespace does not re-enqueue the Agent.
func namespaceFlipRequests(cache cache.Cache, r *Reconciler) func(context.Context, *corev1.Namespace) []reconcile.Request {
	return func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
		list := r.AssociatedObjListTemplate()
		// List **cluster-wide** from the cache (not the FilterClient): associated resources
		// referencing the flipped namespace can live in any matched namespace, and
		// resources in the namespace being de-scoped would be hidden by the FilterClient.
		if err := cache.List(ctx, list); err != nil {
			return nil
		}
		items, err := apimeta.ExtractList(list)
		if err != nil {
			return nil
		}
		var reqs []reconcile.Request
		for _, item := range items {
			associated, ok := item.(commonv1.Associated)
			if !ok {
				continue
			}

			// case A: Reconcile the associated resources that their own namespace changed state.
			if associated.GetNamespace() == ns.Name {
				reqs = append(reqs, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(associated)})
				continue
			}

			// case B: Reconcile the associated resources that at least one of their associations belong to the namespace that changed state.
			for _, association := range associated.GetAssociations() {
				// omit the ones that belong to another association controller
				if association.AssociationType() != r.AssociationType {
					continue
				}
				// AssociationRef() resolves an empty ref namespace to the associated
				// resource's namespace, so the comparison is direct.
				if association.AssociationRef().NamespacedName().Namespace == ns.Name {
					reqs = append(reqs, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(associated)})
					break
				}
			}
		}
		return reqs
	}
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
