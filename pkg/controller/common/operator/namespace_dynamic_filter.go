// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// NamespaceFilter caches namespace selector matches in-memory and can be updated dynamically.
type NamespaceFilter struct {
	selector              labels.Selector
	configuredNamespaces  map[string]struct{}
	managedNamespaces     map[string]struct{}
	managedNamespacesLock sync.RWMutex
}

func NewNamespaceFilter(
	namespaceLabelSelector *metav1.LabelSelector,
	configuredNamespaces []string,
	initialManagedNamespaces []string,
) (*NamespaceFilter, error) {
	selector, err := metav1.LabelSelectorAsSelector(namespaceLabelSelector)
	if err != nil {
		return nil, err
	}

	configured := make(map[string]struct{}, len(configuredNamespaces))
	for _, namespace := range configuredNamespaces {
		if namespace == "" {
			continue
		}
		configured[namespace] = struct{}{}
	}

	managed := make(map[string]struct{}, len(initialManagedNamespaces))
	for _, namespace := range initialManagedNamespaces {
		if namespace == "" {
			continue
		}
		managed[namespace] = struct{}{}
	}

	return &NamespaceFilter{
		selector:             selector,
		configuredNamespaces: configured,
		managedNamespaces:    managed,
	}, nil
}

func (f *NamespaceFilter) ShouldManage(namespace string) bool {
	if namespace == "" {
		return false
	}

	f.managedNamespacesLock.RLock()
	_, exists := f.managedNamespaces[namespace]
	f.managedNamespacesLock.RUnlock()
	return exists
}

func (f *NamespaceFilter) OnNamespaceUpsert(namespace corev1.Namespace) {
	if namespace.Name == "" {
		return
	}

	if !f.isConfiguredNamespace(namespace.Name) {
		f.managedNamespacesLock.Lock()
		delete(f.managedNamespaces, namespace.Name)
		f.managedNamespacesLock.Unlock()
		return
	}

	matches := f.selector.Matches(labels.Set(namespace.Labels))

	f.managedNamespacesLock.Lock()
	defer f.managedNamespacesLock.Unlock()

	if matches {
		f.managedNamespaces[namespace.Name] = struct{}{}
		return
	}

	delete(f.managedNamespaces, namespace.Name)
}

func (f *NamespaceFilter) OnNamespaceDelete(namespace string) {
	if namespace == "" {
		return
	}

	f.managedNamespacesLock.Lock()
	delete(f.managedNamespaces, namespace)
	f.managedNamespacesLock.Unlock()
}

func (f *NamespaceFilter) isConfiguredNamespace(namespace string) bool {
	if len(f.configuredNamespaces) == 0 {
		return true
	}
	_, configured := f.configuredNamespaces[namespace]
	return configured
}
