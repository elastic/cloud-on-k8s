// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

const testOperatorNS = "elastic-system"

func mustSelector(t *testing.T, matchLabels map[string]string) labels.Selector {
	t.Helper()
	ls := metav1.LabelSelector{MatchLabels: matchLabels}
	sel, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		t.Fatalf("LabelSelectorAsSelector error %s", err.Error())
	}
	return sel
}

func namespace(name string, lbls map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: lbls},
	}
}

func TestMatcher(t *testing.T) {
	t.Run("SelectorEnabled", func(t *testing.T) {
		assert.False(t, (*NamespaceFlipNotifier)(nil).SelectorEnabled(), "nil MatchNotifier is disabled")
		assert.False(t, NewMatchNotifier(nil, testOperatorNS).SelectorEnabled(), "nil selector is disabled")
		assert.True(t, NewMatchNotifier(mustSelector(t, map[string]string{"env": "prod"}), testOperatorNS).SelectorEnabled(), "non-nil selector is enabled")
	})

	t.Run("Matches disabled always returns true", func(t *testing.T) {
		m := NewMatchNotifier(nil, testOperatorNS)
		assert.True(t, m.Matches("any-namespace"), "disabled matcher always matches")
		assert.True(t, m.Matches(""), "disabled matcher matches empty namespace")
	})

	t.Run("Matches unknown namespace returns false", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		assert.False(t, m.Matches("unknown"), "namespace not yet in states returns false")
	})

	t.Run("Matches returns true when state is true", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		m.Swap("prod-ns", true)
		assert.True(t, m.Matches("prod-ns"))
	})

	t.Run("Matches returns false when state is false", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		m.Swap("dev-ns", false)
		assert.False(t, m.Matches("dev-ns"))
	})

	t.Run("Matches returns true when state for short-circuit", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		assert.True(t, m.Matches(testOperatorNS))
	})

	t.Run("Matches returns true for empty string short-circuit when selector is enabled", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		assert.True(t, m.Matches(""), "empty namespace is always short-circuited")
	})
}

func TestObserveNamespace(t *testing.T) {
	t.Run("disabled returns true true for any namespace", func(t *testing.T) {
		m := NewMatchNotifier(nil, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace("any", map[string]string{"env": "dev"}))
		assert.True(t, isMatching)
		assert.True(t, wasMatching)
	})

	t.Run("short-circuit empty namespace returns true true without updating state", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace("", nil))
		assert.True(t, isMatching)
		assert.True(t, wasMatching)
		// The short-circuit must not pollute the states map.
		_, inStates := m.matchedNamespaces[""]
		assert.False(t, inStates, "empty namespace must not be written to states")
	})

	t.Run("short-circuit operator namespace returns true true without updating state", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace(testOperatorNS, nil))
		assert.True(t, isMatching)
		assert.True(t, wasMatching)
		_, inStates := m.matchedNamespaces[testOperatorNS]
		assert.False(t, inStates, "operator namespace must not be written to states")
	})

	t.Run("matching namespace updates state and returns correct values", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace("prod-ns", map[string]string{"env": "prod"}))
		assert.True(t, isMatching)
		assert.False(t, wasMatching, "not previously known")
		assert.True(t, m.Matches("prod-ns"), "state updated to true")
	})

	t.Run("non-matching namespace updates state and returns correct values", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace("dev-ns", map[string]string{"env": "dev"}))
		assert.False(t, isMatching)
		assert.False(t, wasMatching, "not previously known")
		assert.False(t, m.Matches("dev-ns"), "state updated to false")
	})

	t.Run("namespace transitions from matching to non-matching", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		m.ObserveNamespace(namespace("ns", map[string]string{"env": "prod"}))

		isMatching, wasMatching := m.ObserveNamespace(namespace("ns", map[string]string{"env": "staging"}))
		assert.False(t, isMatching)
		assert.True(t, wasMatching)
	})

	t.Run("namespace transitions from non-matching to matching", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		m.ObserveNamespace(namespace("ns", map[string]string{"env": "dev"}))

		isMatching, wasMatching := m.ObserveNamespace(namespace("ns", map[string]string{"env": "prod"}))
		assert.True(t, isMatching)
		assert.False(t, wasMatching)
	})
}

func TestNotifier(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})
	ctx := context.Background()

	t.Run("no subscribers", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		// Broadcast with no subscribers must not panic.
		n.Broadcast(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
	})

	t.Run("disabled selector no-ops broadcast", func(t *testing.T) {
		n := NewMatchNotifier(nil, "")
		ch := n.Subscribe()
		n.Broadcast(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
		assert.Len(t, ch, 0, "broadcast is a no-op when selector is disabled")
	})

	t.Run("single subscriber receives broadcast", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ch := n.Subscribe()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}}
		n.Broadcast(ctx, ns)

		require.Len(t, ch, 1)
		assert.Equal(t, ns, (<-ch).Object)
	})

	t.Run("multiple subscribers each receive broadcast", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ch1 := n.Subscribe()
		ch2 := n.Subscribe()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}}
		n.Broadcast(ctx, ns)

		require.Len(t, ch1, 1)
		require.Len(t, ch2, 1)
		assert.Equal(t, ns, (<-ch1).Object)
		assert.Equal(t, ns, (<-ch2).Object)
	})

	t.Run("multiple events delivered in order", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ch := n.Subscribe()

		ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}}
		ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-2"}}
		n.Broadcast(ctx, ns1)
		n.Broadcast(ctx, ns2)

		require.Len(t, ch, 2)
		assert.Equal(t, ns1, (<-ch).Object)
		assert.Equal(t, ns2, (<-ch).Object)
	})

	t.Run("late subscriber does not receive earlier broadcasts", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c"}}
		n.Broadcast(ctx, ns) // no subscribers yet

		ch := n.Subscribe()
		assert.Len(t, ch, 0, "late subscriber must not receive events broadcast before it subscribed")
	})

	t.Run("cancelled context does not block on full subscriber channel", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		n.subscriberBufferSize = 0
		_ = n.Subscribe()
		// Access the underlying bidirectional channel.
		internal := n.subs[0]

		done := make(chan struct{})
		go func() {
			cancelledCtx, cancel := context.WithCancel(t.Context())
			cancel()
			n.Broadcast(cancelledCtx, namespace("ns", nil))
			close(done)
		}()

		select {
		case <-done:
			// expected: Broadcast returned without blocking
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Broadcast blocked despite cancelled context")
		}
		assert.Len(t, internal, n.subscriberBufferSize, "no new event must be added to an already-full channel")
	})

	t.Run("full subscriber channel", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		n.subscriberSendTimeout = 20 * time.Millisecond
		n.subscriberBufferSize = 1
		_ = n.Subscribe() // first: will be full → send times out
		_ = n.Subscribe() // second: must still receive the event
		first, second := n.subs[0], n.subs[1]

		for range n.subscriberBufferSize {
			first <- event.TypedGenericEvent[*corev1.Namespace]{}
		}

		done := make(chan struct{})
		go func() {
			err := n.Broadcast(context.Background(), namespace("ns", nil))
			assert.Error(t, err)
			close(done)
		}()

		select {
		case <-done:
			// expected: Broadcast returned after the per-subscriber timeout fired
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Broadcast blocked despite send timeout")
		}

		require.Len(t, second, 1, "second subscriber must receive event despite first timing out")
		assert.Len(t, first, n.subscriberBufferSize, "full subscriber must not gain a new event")
	})

	t.Run("cancelled context", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		// Make channels non-buffered so that ch <- item cannot win the select race,
		// guaranteeing context.Canceled is always returned for each subscriber.
		n.subscriberBufferSize = 0
		_ = n.Subscribe()
		_ = n.Subscribe()

		cancelledCtx, cancel := context.WithCancel(t.Context())
		cancel()

		done := make(chan struct{})
		var broadcastErr error
		go func() {
			broadcastErr = n.Broadcast(cancelledCtx, namespace("ns", nil))
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Broadcast blocked despite cancelled context")
		}

		require.Error(t, broadcastErr)
		var joinedErrs interface{ Unwrap() []error }
		require.ErrorAs(t, broadcastErr, &joinedErrs)
		errs := joinedErrs.Unwrap()
		require.Len(t, errs, 2, "expected one error per subscriber")
		assert.ErrorIs(t, errs[0], context.Canceled)
		assert.ErrorIs(t, errs[1], context.Canceled)
	})
}

func TestObserveAndBroadcast(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})
	ctx := context.Background()

	matching := namespace("prod-ns", map[string]string{"env": "prod"})
	nonMatching := namespace("dev-ns", map[string]string{"env": "dev"})

	t.Run("disabled selector: no state change, no broadcast, reports matching", func(t *testing.T) {
		m := NewMatchNotifier(nil, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, matching)
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("short-circuit namespace: no state change, no broadcast, reports matching", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, namespace(testOperatorNS, nil))
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("first observe, namespace matches: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, matching)
		assert.True(t, stateChanged)
		assert.True(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, matching, (<-ch).Object)
	})

	t.Run("first observe, namespace does not match: no state change, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, nonMatching)
		assert.False(t, stateChanged)
		assert.False(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state unchanged: namespace still matches, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(matching.Name, true)
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, matching)
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state unchanged: namespace still does not match, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(nonMatching.Name, false)
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, nonMatching)
		assert.False(t, stateChanged)
		assert.False(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state change matching -> non-matching: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(nonMatching.Name, true) // was matching
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, nonMatching)
		assert.True(t, stateChanged)
		assert.False(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, nonMatching, (<-ch).Object)
	})

	t.Run("state change non-matching -> matching: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(matching.Name, false) // was non-matching
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, matching)
		assert.True(t, stateChanged)
		assert.True(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, matching, (<-ch).Object)
	})

	t.Run("empty namespace short-circuit: no state change, no broadcast, reports matching", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching, _ := m.ObserveAndBroadcast(ctx, namespace("", nil))
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})
}

func TestSwap(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("first swap true: wasMatching false, namespace added to states", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		wasMatching := m.Swap("ns", true)
		assert.False(t, wasMatching)
		assert.True(t, m.Matches("ns"))
	})

	t.Run("second swap true: wasMatching true, namespace stays in states", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		m.Swap("ns", true)
		wasMatching := m.Swap("ns", true)
		assert.True(t, wasMatching)
		assert.True(t, m.Matches("ns"))
	})

	t.Run("swap false when was matching: wasMatching true, namespace removed from states", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		m.Swap("ns", true)
		wasMatching := m.Swap("ns", false)
		assert.True(t, wasMatching)
		assert.False(t, m.Matches("ns"))
	})

	t.Run("swap false when was not tracked: wasMatching false, namespace absent from states", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		wasMatching := m.Swap("ns", false)
		assert.False(t, wasMatching)
		assert.False(t, m.Matches("ns"))
	})
}
