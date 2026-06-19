// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const chanSize = 32

// NamespaceFlipNotifier evaluates a label selector against namespace labels,
// tracks each namespace's match state, and broadcasts to subscribers whenever
// that state flips.
//
// When the selector is nil (disabled) the notifier is a no-op: Matches always
// returns true and Broadcast is a no-op. This preserves legacy / static-
// resolution behaviour without any code changes in callers.
//
// Two namespaces bypass selector evaluation in ObserveNamespace: the empty
// string (cluster-scoped events) and the operator's own namespace — both
// always return true so the operator can reconcile its own resources
// regardless of the configured selector.
//
// Matches returns false for any namespace whose state has not yet been
// recorded by ObserveNamespace. The namespace flip-state controller seeds
// all existing namespaces at startup via initiateNamespaces and re-enqueues
// CRs whenever a namespace's match state changes.
type NamespaceFlipNotifier struct {
	selector     labels.Selector
	shortCircuit map[string]struct{} // namespaces excluded from label-selector evaluation.
	subs         []chan event.TypedGenericEvent[*corev1.Namespace]
	statesMutex  sync.Mutex
	states       map[string]bool // namespace name -> last known match result
}

// NewMatchNotifier returns a NamespaceFlipNotifier. When sel is nil it acts as
// a no-op (Matches always returns true). operatorNS is always added to the
// short-circuit set so the operator can reconcile its own namespace regardless
// of the configured selector.
func NewMatchNotifier(sel labels.Selector, operatorNS string) *NamespaceFlipNotifier {
	return &NamespaceFlipNotifier{
		selector: sel,
		shortCircuit: map[string]struct{}{
			"":         {},
			operatorNS: {},
		},
		states: map[string]bool{},
	}
}

// SelectorEnabled reports whether the matcher is actively filtering.
func (m *NamespaceFlipNotifier) SelectorEnabled() bool {
	return m != nil && m.selector != nil
}

// Matches returns the last recorded match state for ns. It returns false for
// any namespace not yet observed by ObserveNamespace. When the selector is
// disabled, always returns true.
func (m *NamespaceFlipNotifier) Matches(ns string) bool {
	if !m.SelectorEnabled() {
		return true
	}

	m.statesMutex.Lock()
	defer m.statesMutex.Unlock()
	return m.states[ns]
}

// ObserveNamespace evaluates the selector against ns's current labels, records
// the result, and returns both the current and previous match states. Short-
// circuited namespaces (empty string and the operator's namespace) always
// return (true, true) without updating the internal state map. When the
// selector is disabled, always returns (true, true).
func (m *NamespaceFlipNotifier) ObserveNamespace(ns *corev1.Namespace) (isMatching bool, wasMatching bool) {
	if !m.SelectorEnabled() {
		return true, true
	}

	if _, ok := m.shortCircuit[ns.Name]; ok {
		return true, true
	}

	isMatching = m.selector.Matches(labels.Set(ns.Labels))

	wasMatching = m.Swap(ns.Name, isMatching)
	return
}

// Subscribe returns a receive-only channel that receives one event each time
// Broadcast is called. Events are dropped silently when the buffer is full.
// Intended to be passed to a controller's Watch source.
func (m *NamespaceFlipNotifier) Subscribe() <-chan event.TypedGenericEvent[*corev1.Namespace] {
	ch := make(chan event.TypedGenericEvent[*corev1.Namespace], chanSize)
	// No mutex needed: all controllers call Subscribe sequentially during
	// manager initialization, before mgr.Start() launches any goroutines.
	m.subs = append(m.subs, ch)
	return ch
}

// Broadcast sends ns to every subscriber. Drops the event for any subscriber
// whose channel buffer is currently full.
func (m *NamespaceFlipNotifier) Broadcast(ns *corev1.Namespace) {
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

// ObserveAndBroadcast calls ObserveNamespace and broadcasts ns to all
// subscribers if the match state changed. Returns whether the state changed
// and the current match result.
func (m *NamespaceFlipNotifier) ObserveAndBroadcast(ns *corev1.Namespace) (stateChanged, isMatching bool) {
	var wasWatching bool
	isMatching, wasWatching = m.ObserveNamespace(ns)
	stateChanged = isMatching != wasWatching
	if stateChanged {
		m.Broadcast(ns)
	}
	return
}

// Swap records isMatching for ns and returns the previously recorded value
// (wasMatching).
func (m *NamespaceFlipNotifier) Swap(ns string, isMatching bool) (wasMatching bool) {
	m.statesMutex.Lock()
	defer m.statesMutex.Unlock()
	wasMatching = m.states[ns]
	m.states[ns] = isMatching
	return
}

// ForgetNamespace removes the recorded state for a namespace, typically when it is
// deleted.
func (m *NamespaceFlipNotifier) ForgetNamespace(ns string) {
	m.statesMutex.Lock()
	defer m.statesMutex.Unlock()
	delete(m.states, ns)
}
