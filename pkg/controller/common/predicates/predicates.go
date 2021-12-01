// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package predicates

import (
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ManagedNamespacePredicate is a predicate that will ignore events that exist
// outside of the namespaces that the operator manages.
var ManagedNamespacePredicate predicate.Predicate

// NewManagedNamespacesPredicate will return a predicate that will ignore events
// that exist outside of the given managed namespaces,
func NewManagedNamespacesPredicate(managedNamespaces []string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return IsNamespaceManaged(e.Object.GetNamespace(), managedNamespaces)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return IsNamespaceManaged(e.ObjectNew.GetNamespace(), managedNamespaces)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return IsNamespaceManaged(e.Object.GetNamespace(), managedNamespaces)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return namespaceInSlice(e.Object.GetNamespace(), managedNamespaces)
		},
	}
}

// IsNamespaceManaged returns true if the namespace is managed by the operator.
func IsNamespaceManaged(namespace string, managedNamespaces []string) bool {
	return len(managedNamespaces) == 0 || slices.Contains(managedNamespaces, namespace)
}

// WithPredicates is a helper function to convert one or more predicates
// into a slice of predicates.
func WithPredicates(predicates ...predicate.Predicate) []predicate.Predicate {
	return predicates
}
