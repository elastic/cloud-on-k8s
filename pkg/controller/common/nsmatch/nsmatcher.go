// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

type NamespaceMatcher struct {
	selector                 labels.Selector
	alwaysManagedNamespaces  map[string]struct{} // namespaces excluded from label-selector evaluation.
	cache                    cache.Cache
	preRegisterInformerCache bool
}

func NewNamespaceMatcher(sel labels.Selector, operatorNS string, preRegisterInformerCache bool) *NamespaceMatcher {
	return &NamespaceMatcher{
		selector: sel,
		alwaysManagedNamespaces: map[string]struct{}{
			"":         {},
			operatorNS: {},
		},
		preRegisterInformerCache: preRegisterInformerCache,
	}
}

func (m *NamespaceMatcher) SetCache(c cache.Cache) {
	m.cache = c
}

// SelectorEnabled reports whether the matcher is actively filtering.
// Safe to call on a nil receiver; returns false in that case.
func (m *NamespaceMatcher) SelectorEnabled() bool {
	return m != nil && m.selector != nil
}

func (m *NamespaceMatcher) NamespaceNameMatches(ctx context.Context, ns string) (bool, error) {
	if !m.SelectorEnabled() {
		return true, nil
	}

	if _, ok := m.alwaysManagedNamespaces[ns]; ok {
		return true, nil
	}

	namespace := &corev1.Namespace{}
	if err := m.cache.Get(ctx, types.NamespacedName{Name: ns}, namespace); err != nil {
		return false, err
	}

	return m.NamespaceMatches(namespace), nil
}

func (m *NamespaceMatcher) NamespaceMatches(ns *corev1.Namespace) bool {
	if !m.SelectorEnabled() {
		return true
	}

	if _, ok := m.alwaysManagedNamespaces[ns.Name]; ok {
		return true
	}

	return m.selector.Matches(labels.Set(ns.Labels))
}

func (m *NamespaceMatcher) MatchingNamespaces(ctx context.Context) ([]string, error) {
	if !m.SelectorEnabled() {
		return nil, nil
	}

	var list corev1.NamespaceList
	if err := m.cache.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("error while listing all namespaces: %w", err)
	}

	seen := make(map[string]struct{}, len(list.Items))
	for i := range list.Items {
		ns := &list.Items[i]
		if m.NamespaceMatches(ns) {
			seen[ns.Name] = struct{}{}
		}
	}
	for ns := range m.alwaysManagedNamespaces {
		if ns == "" {
			continue
		}
		seen[ns] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}

	return names, nil
}

func (m *NamespaceMatcher) PreRegisterInformerCache() bool {
	if m == nil {
		return false
	}
	return m.preRegisterInformerCache
}
