// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func namespaceInSlice(namespace string, namespaces []string) bool {
	// If the operator is managing all namespaces,
	// never ignore any namespace.
	if len(namespaces) == 0 {
		return true
	}
	for _, ns := range namespaces {
		if namespace == ns {
			return true
		}
	}
	return false
}

// WithPredicates is a helper function to convert one or more predicates
// into a slice of predicates.
func WithPredicates(predicates ...predicate.Predicate) []predicate.Predicate {
	return predicates
}

// ManagedNamespacesPredicate will return a predicate that will ignore events
// that exist outside of the given managed namespaces,
func ManagedNamespacesPredicate(managedNamespaces []string) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return namespaceInSlice(e.Object.GetNamespace(), managedNamespaces)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return namespaceInSlice(e.ObjectNew.GetNamespace(), managedNamespaces)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Ignore resources that do not exist within the managed namespaces
			return namespaceInSlice(e.Object.GetNamespace(), managedNamespaces)
		},
	}
}
