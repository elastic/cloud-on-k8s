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
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/go-logr/logr"

	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

// subscriberBufferSize is the buffer depth for each subscriber channel. A buffer this size
// lets the broadcaster proceed without blocking during bursts of namespace events.
const subscriberBufferSize = 32

const subscriberLogBackpressureDuration = 3 * time.Second

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
// CRs whenever a namespace's match state changes. Seeding runs concurrently
// with the controllers (the manager does not order runnables), so events
// dropped while a namespace is not yet seeded are backfilled by the broadcast
// its seeding emits.
//
// The match-state map is maintained on every operator replica (the namespace
// controller and its seeder do not require leader election) so that consumers
// that run on non-leaders — the webhook server and the namespace-filtering
// client — see the same state as the leader. Broadcasting to subscribers, in
// contrast, only happens on the elected leader; see Broadcast and SetElected.
type NamespaceMatcher struct {
	cache                             cache.Cache
	selector                          labels.Selector
	alwaysManagedNamespaces           map[string]struct{} // namespaces excluded from label-selector evaluation.
	subs                              []chan event.TypedGenericEvent[*corev1.Namespace]
	matchedNamespacesMutex            sync.Mutex
	matchedNamespaces                 map[string]struct{} // namespace name -> present if namespace matches to the selector
	subscriberLogBackpressureDuration time.Duration
	subscriberBufferSize              int
	elected                           <-chan struct{} // closed once this replica is elected leader; nil means always elected.
}

// NewNamespaceMatcher returns a NamespaceMatcher. When sel is nil it acts as
// a no-op (Matches always returns true). Both the empty string (cluster-scoped
// resources) and operatorNS are pre-seeded in the short-circuit set so that
// cluster-scoped events and the operator's own namespace always match regardless
// of the configured selector.
func NewNamespaceMatcher(sel labels.Selector, operatorNS string) *NamespaceMatcher {
	return &NamespaceMatcher{
		selector: sel,
		alwaysManagedNamespaces: map[string]struct{}{
			"":         {},
			operatorNS: {},
		},
		matchedNamespaces:                 map[string]struct{}{},
		subscriberLogBackpressureDuration: subscriberLogBackpressureDuration,
		subscriberBufferSize:              subscriberBufferSize,
	}
}

func (m *NamespaceMatcher) SetCache(c cache.Cache) {
	m.cache = c
}

// SetElected provides the manager's election signal (manager.Elected()), a
// channel that is closed once this replica becomes leader (or immediately when
// leader election is disabled). The match-state map is maintained on every
// replica, but the controllers subscribed via Subscribe are leader-election
// runnables: on a non-leader they never consume from their channels, so
// Broadcast must not send. When set, Broadcast drops events until the channel
// is closed. When never set (tests), Broadcast always sends.
func (m *NamespaceMatcher) SetElected(elected <-chan struct{}) {
	m.elected = elected
}

// isElected reports whether this replica may broadcast to subscribers. True
// when no election signal was provided or when the election channel is closed.
//
// The select is deterministic: default only runs when no other case can
// proceed, and a receive from a closed channel always proceeds, so once
// elected is closed this never returns false. A select racing with the close
// itself may still return false; that at worst drops one broadcast at the
// moment of election, which the subscribers' initial sync covers anyway.
func (m *NamespaceMatcher) isElected() bool {
	if m.elected == nil {
		return true
	}
	select {
	case <-m.elected:
		return true
	default:
		return false
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

func (m *NamespaceMatcher) MatchingNamespaces() []string {
	m.matchedNamespacesMutex.Lock()
	defer m.matchedNamespacesMutex.Unlock()

	names := make([]string, 0, len(m.matchedNamespaces)+len(m.alwaysManagedNamespaces))
	for ns := range m.matchedNamespaces {
		names = append(names, ns)
	}
	for ns := range m.alwaysManagedNamespaces {
		if ns == "" {
			continue
		}
		names = append(names, ns)
	}

	return names
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

// Broadcast sends the namespace to every subscriber, blocking on each send
// until it succeeds or ctx is done. A blocked send is logged periodically
// (every subscriberLogBackpressureDuration) so persistent backpressure is
// visible. On context cancellation, Broadcast stops waiting on the current
// subscriber, records ctx's error for it, and moves on to the rest.
//
// On a replica that has not (yet) been elected leader, Broadcast is a no-op:
// subscriber controllers only run on the leader, so nothing consumes from the
// channels and a blocking send would never complete. Dropping is safe because
// when this replica is later elected, every subscriber controller starts with
// a full initial sync of its own CRs, whose watch predicates consult the
// match-state map — maintained on every replica — so flips dropped here are
// re-evaluated then.
func (m *NamespaceMatcher) Broadcast(ctx context.Context, ns *corev1.Namespace) error {
	if !m.SelectorEnabled() {
		return nil
	}

	if !m.isElected() {
		// Dropping the event here loses no information: the match state was
		// recorded in the map before Broadcast was called, and the map is what
		// carries the state across the election. When this replica becomes
		// leader, the elected channel is closed before the subscriber
		// controllers start, so their initial sync replays every CR against
		// the already-warm map — re-enqueue triggers dropped here are subsumed
		// by that replay. Broadcasts are only needed for flips that happen
		// after the initial sync, and by then this branch is no longer taken.
		return nil
	}

	log := ulog.FromContext(ctx)

	// No mutex needed: subs is written only during initialization (Subscribe)
	// and is immutable by the time Broadcast is first called. Concurrent sends
	// to the same channel are safe because channels handle their own synchronization.
	ev := event.TypedGenericEvent[*corev1.Namespace]{Object: ns}
	errs := make([]error, len(m.subs))
	for i, ch := range m.subs {
		errs[i] = sendAndLogBackpressure(ctx, log, ch, m.subscriberLogBackpressureDuration, ev)
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
// (wasMatching). It is intended for internal use by ObserveNamespace and for
// external test usage; other callers should use ObserveNamespace and
// ForgetNamespace instead.
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

// ForgetNamespace clears any recorded match state for ns without broadcasting.
// Intended for use when ns has been deleted: its resources are being cleaned
// up by their own controllers, so there is nothing for subscribers to react to.
func (m *NamespaceMatcher) ForgetNamespace(ns string) {
	_ = m.Swap(ns, false)
}

func sendAndLogBackpressure[chType any](ctx context.Context, log logr.Logger, ch chan<- chType, logDuration time.Duration, item chType) error {
	ticker := time.NewTicker(logDuration)
	defer ticker.Stop()
	waited := time.Duration(0)
	for {
		select {
		case ch <- item:
			return nil
		case <-ticker.C:
			waited += logDuration
			log.Info("subscriber channel send is still blocked, possible backpressure", "waited", waited.String())
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
