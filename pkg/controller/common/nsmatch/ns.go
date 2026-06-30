// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"errors"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// subscriberBufferSize is the buffer depth for each subscriber channel. A buffer this size
// lets the broadcaster proceed without blocking during bursts of namespace events.
const subscriberBufferSize = 32

// subscriberSendTimeout is the per-subscriber send deadline used by Broadcast.
// Declared as a var so tests can override it without waiting 3 seconds.
const subscriberSendTimeout = 3 * time.Second

// NamespaceMatcher evaluates a label selector against namespace labels,
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
// all existing namespaces at startup and re-enqueues
// CRs whenever a namespace's match state changes.
type NamespaceMatcher struct {
	selector                labels.Selector
	alwaysManagedNamespaces map[string]struct{} // namespaces excluded from label-selector evaluation.
	subs                    []chan event.TypedGenericEvent[*corev1.Namespace]
	matchedNamespacesMutex  sync.Mutex
	matchedNamespaces       map[string]struct{} // namespace name -> present if namespace matches to the selector
	subscriberSendTimeout   time.Duration
	subscriberBufferSize    int
}

// NewMatchNotifier returns a NamespaceFlipNotifier. When sel is nil it acts as
// a no-op (Matches always returns true). Both the empty string (cluster-scoped
// resources) and operatorNS are pre-seeded in the short-circuit set so that
// cluster-scoped events and the operator's own namespace always match regardless
// of the configured selector.
func NewMatchNotifier(sel labels.Selector, operatorNS string) *NamespaceMatcher {
	return &NamespaceMatcher{
		selector: sel,
		alwaysManagedNamespaces: map[string]struct{}{
			"":         {},
			operatorNS: {},
		},
		matchedNamespaces:     map[string]struct{}{},
		subscriberSendTimeout: subscriberSendTimeout,
		subscriberBufferSize:  subscriberBufferSize,
	}
}

// SelectorEnabled reports whether the matcher is actively filtering.
// Safe to call on a nil receiver; returns false in that case.
func (m *NamespaceMatcher) SelectorEnabled() bool {
	return m != nil && m.selector != nil
}

// Matches returns the last recorded match state for ns. It returns false for
// any namespace not yet observed by ObserveNamespace. When the selector is
// disabled, always returns true.
func (m *NamespaceMatcher) Matches(ns string) bool {
	if !m.SelectorEnabled() {
		return true
	}

	if _, ok := m.alwaysManagedNamespaces[ns]; ok {
		return true
	}

	m.matchedNamespacesMutex.Lock()
	defer m.matchedNamespacesMutex.Unlock()
	_, match := m.matchedNamespaces[ns]
	return match
}

// ObserveNamespace evaluates the selector against ns's current labels, records
// the result, and returns both the current and previous match states. Short-
// circuited namespaces (empty string and the operator's namespace) always
// return (true, true) without updating the internal state map. When the
// selector is disabled, always returns (true, true).
func (m *NamespaceMatcher) ObserveNamespace(ns *corev1.Namespace) (isMatching bool, wasMatching bool) {
	if !m.SelectorEnabled() {
		return true, true
	}

	if _, ok := m.alwaysManagedNamespaces[ns.Name]; ok {
		return true, true
	}

	isMatching = m.selector.Matches(labels.Set(ns.Labels))

	wasMatching = m.Swap(ns.Name, isMatching)
	return
}

// Subscribe returns a receive-only channel that receives one event each time
// Broadcast is called. Intended to be passed to a controller's Watch source.
func (m *NamespaceMatcher) Subscribe() <-chan event.TypedGenericEvent[*corev1.Namespace] {
	ch := make(chan event.TypedGenericEvent[*corev1.Namespace], m.subscriberBufferSize)
	// No mutex needed: all controllers call Subscribe sequentially during
	// manager initialization, before mgr.Start() launches any goroutines.
	m.subs = append(m.subs, ch)
	return ch
}

// Broadcast sends the namespace to every subscriber using a per-send timeout of
// broadcastTimeout. On timeout the error is logged and delivery continues to the
// remaining subscribers. On context cancellation the error is logged and Broadcast
// returns early.
func (m *NamespaceMatcher) Broadcast(ctx context.Context, ns *corev1.Namespace) error {
	if !m.SelectorEnabled() {
		return nil
	}
	// No mutex needed: subs is written only during initialization (Subscribe)
	// and is immutable by the time Broadcast is first called. Concurrent sends
	// to the same channel are safe because channels handle their own synchronization.
	ev := event.TypedGenericEvent[*corev1.Namespace]{Object: ns}
	errs := make([]error, len(m.subs))
	for i, ch := range m.subs {
		errs[i] = sendWithTimeout(ctx, ch, m.subscriberSendTimeout, ev)
	}
	return errors.Join(errs...)
}

// ObserveAndBroadcast calls ObserveNamespace and broadcasts ns to all
// subscribers if the match state flipped. Returns whether the state changed
// and the current match result. When the selector is disabled, ObserveNamespace
// returns (true, true), so stateChanged is always false and no broadcast occurs.
func (m *NamespaceMatcher) ObserveAndBroadcast(ctx context.Context, ns *corev1.Namespace) (stateChanged, isMatching bool, err error) {
	var wasWatching bool
	isMatching, wasWatching = m.ObserveNamespace(ns)
	stateChanged = isMatching != wasWatching
	if stateChanged {
		err = m.Broadcast(ctx, ns)
	}
	return
}

// Swap records isMatching for ns and returns the previously recorded value
// (wasMatching).
func (m *NamespaceMatcher) Swap(ns string, isMatching bool) (wasMatching bool) {
	m.matchedNamespacesMutex.Lock()
	defer m.matchedNamespacesMutex.Unlock()
	_, wasMatching = m.matchedNamespaces[ns]
	if isMatching {
		m.matchedNamespaces[ns] = struct{}{}
	} else {
		delete(m.matchedNamespaces, ns)
	}
	return
}

func sendWithTimeout[chType any](ctx context.Context, ch chan<- chType, timeout time.Duration, item chType) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case ch <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
