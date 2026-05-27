// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const chanSize = 32

// MatchNotifier evaluates a label selector against a Namespace's current labels
// and broadcasts match-state changes to subscribers.
//
// A disabled MatchNotifier (nil selector) matches every namespace (legacy /
// static-resolution modes). Two namespaces always bypass the selector: the
// empty string (cluster-scoped events) and the operator's own namespace.
//
// On a cache miss Matches returns false — safer to drop an event than act on a
// namespace outside the configured scope. The Namespace flip-state controller
// re-enqueues CRs when a namespace becomes visible/matching.
type MatchNotifier struct {
	cache        cache.Cache
	selector     labels.Selector
	shortCircuit map[string]struct{} // namespaces excluded from label-selector evaluation.
	subs         []chan event.TypedGenericEvent[*corev1.Namespace]
}

// NewMatchNotifier returns a MatchNotifier. When sel is nil it acts as a no-op
// (Matches always returns true). operatorNS is always added to the short-circuit
// set so the operator can reconcile its own namespace regardless of the selector.
func NewMatchNotifier(c cache.Cache, sel labels.Selector, operatorNS string) *MatchNotifier {
	return &MatchNotifier{
		cache:    c,
		selector: sel,
		shortCircuit: map[string]struct{}{
			"":         {},
			operatorNS: {},
		},
	}
}

// SelectorEnabled reports whether the matcher is actively filtering.
func (m *MatchNotifier) SelectorEnabled() bool {
	return m != nil && m.selector != nil
}

// Matches returns true if the namespace's current labels satisfy the configured
// selector. The empty namespace and the operator's own namespace always match.
// When the selector is disabled every call returns true.
func (m *MatchNotifier) Matches(ctx context.Context, ns string) bool {
	if !m.SelectorEnabled() {
		return true
	}
	if _, ok := m.shortCircuit[ns]; ok {
		return true
	}
	var nsObj corev1.Namespace
	if err := m.cache.Get(ctx, client.ObjectKey{Name: ns}, &nsObj); err != nil {
		return false
	}
	return m.selector.Matches(labels.Set(nsObj.Labels))
}

// MatchesLabels evaluates the selector against the given label set without
// touching the cache. Useful in the Namespace flip-state controller where
// the post-change labels are already on the event object.
func (m *MatchNotifier) MatchesLabels(lbls map[string]string) bool {
	if !m.SelectorEnabled() {
		return true
	}
	return m.selector.Matches(labels.Set(lbls))
}

// Subscribe returns a receive-only channel that receives one event each time a
// namespace's match-state flips. Events are dropped silently when the buffer
// is full. It is intended to be used on controllers watch.
func (m *MatchNotifier) Subscribe() <-chan event.TypedGenericEvent[*corev1.Namespace] {
	ch := make(chan event.TypedGenericEvent[*corev1.Namespace], chanSize)
	// No mutex needed: all controllers call Subscribe sequentially during
	// manager initialization, before mgr.Start() launches any goroutines.
	m.subs = append(m.subs, ch)
	return ch
}

// Broadcast sends ns to every subscriber. Drops the event for any subscriber
// whose channel buffer is currently full.
func (m *MatchNotifier) Broadcast(ns *corev1.Namespace) {
	if !m.SelectorEnabled() {
		return
	}
	// No mutex needed: subs is written only during initialization (Subscribe)
	// and is immutable by the time Broadcast is first called. Concurrent sends
	// to the same channel are safe because channels handle their own synchronization.
	for _, ch := range m.subs {
		select {
		case ch <- event.TypedGenericEvent[*corev1.Namespace]{Object: ns}:
		default:
		}
	}
}
