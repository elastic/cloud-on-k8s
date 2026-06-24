// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
		assert.False(t, m.Matches(""), "empty namespace must not be written to states")
	})

	t.Run("short-circuit operator namespace returns true true without updating state", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(sel, testOperatorNS)
		isMatching, wasMatching := m.ObserveNamespace(namespace(testOperatorNS, nil))
		assert.True(t, isMatching)
		assert.True(t, wasMatching)
		assert.False(t, m.Matches(testOperatorNS), "operator namespace must not be written to states")
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

	t.Run("no subscribers", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		// Broadcast with no subscribers must not panic.
		n.Broadcast(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
	})

	t.Run("disabled selector no-ops broadcast", func(t *testing.T) {
		n := NewMatchNotifier(nil, "")
		ch := n.Subscribe()
		n.Broadcast(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
		assert.Len(t, ch, 0, "broadcast is a no-op when selector is disabled")
	})

	t.Run("single subscriber receives broadcast", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ch := n.Subscribe()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}}
		n.Broadcast(ns)

		require.Len(t, ch, 1)
		assert.Equal(t, ns, (<-ch).Object)
	})

	t.Run("multiple subscribers each receive broadcast", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ch1 := n.Subscribe()
		ch2 := n.Subscribe()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}}
		n.Broadcast(ns)

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
		n.Broadcast(ns1)
		n.Broadcast(ns2)

		require.Len(t, ch, 2)
		assert.Equal(t, ns1, (<-ch).Object)
		assert.Equal(t, ns2, (<-ch).Object)
	})

	t.Run("late subscriber does not receive earlier broadcasts", func(t *testing.T) {
		n := NewMatchNotifier(sel, "")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c"}}
		n.Broadcast(ns) // no subscribers yet

		ch := n.Subscribe()
		assert.Len(t, ch, 0, "late subscriber must not receive events broadcast before it subscribed")
	})
}

func TestNamespaceStates(t *testing.T) {
	t.Run("Swap unknown namespace returns not known", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		wasMatching := s.Swap("ns-a", true)
		assert.False(t, wasMatching, "zero value returned for unknown namespace")
	})

	t.Run("Swap known namespace returns previous value", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		s.Swap("ns-a", true) // seed

		wasMatching := s.Swap("ns-a", false)
		assert.True(t, wasMatching, "previous value was true")
	})

	t.Run("Swap same value round-trips correctly", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		s.Swap("ns-a", true)

		wasMatching := s.Swap("ns-a", true)
		assert.True(t, wasMatching)
	})

	t.Run("ForgetNamespace makes namespace unknown again", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		s.Swap("ns-a", true)
		s.ForgetNamespace("ns-a")

		wasMatching := s.Swap("ns-a", false)
		assert.False(t, wasMatching, "after ForgetNamespace the namespace must be non-matching again")
	})

	t.Run("ForgetNamespace unknown namespace does not panic", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		s.ForgetNamespace("never-seen")
	})

	t.Run("independent namespaces do not share state", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		s.Swap("ns-a", true)
		s.Swap("ns-b", false)

		wasA := s.Swap("ns-a", false)
		wasB := s.Swap("ns-b", true)

		assert.True(t, wasA)
		assert.False(t, wasB)
	})

	t.Run("concurrent Swap and ForgetNamespace do not race", func(t *testing.T) {
		s := &NamespaceFlipNotifier{states: map[string]bool{}}
		const goroutines = 10

		var wg sync.WaitGroup
		wg.Add(goroutines * 2)
		for range goroutines {
			go func() {
				defer wg.Done()
				s.Swap("ns-concurrent", true)
			}()
			go func() {
				defer wg.Done()
				s.ForgetNamespace("ns-concurrent")
			}()
		}
		wg.Wait()
	})
}

func TestObserveAndBroadcast(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	matching := namespace("prod-ns", map[string]string{"env": "prod"})
	nonMatching := namespace("dev-ns", map[string]string{"env": "dev"})

	t.Run("disabled selector: no state change, no broadcast, reports matching", func(t *testing.T) {
		m := NewMatchNotifier(nil, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching := m.ObserveAndBroadcast(matching)
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("short-circuit namespace: no state change, no broadcast, reports matching", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching := m.ObserveAndBroadcast(namespace(testOperatorNS, nil))
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("first observe, namespace matches: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching := m.ObserveAndBroadcast(matching)
		assert.True(t, stateChanged)
		assert.True(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, matching, (<-ch).Object)
	})

	t.Run("first observe, namespace does not match: no state change, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		stateChanged, isMatching := m.ObserveAndBroadcast(nonMatching)
		assert.False(t, stateChanged)
		assert.False(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state unchanged: namespace still matches, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(matching.Name, true)
		stateChanged, isMatching := m.ObserveAndBroadcast(matching)
		assert.False(t, stateChanged)
		assert.True(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state unchanged: namespace still does not match, no broadcast", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(nonMatching.Name, false)
		stateChanged, isMatching := m.ObserveAndBroadcast(nonMatching)
		assert.False(t, stateChanged)
		assert.False(t, isMatching)
		assert.Len(t, ch, 0)
	})

	t.Run("state change matching -> non-matching: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(nonMatching.Name, true) // was matching
		stateChanged, isMatching := m.ObserveAndBroadcast(nonMatching)
		assert.True(t, stateChanged)
		assert.False(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, nonMatching, (<-ch).Object)
	})

	t.Run("state change non-matching -> matching: state changed, broadcast sent", func(t *testing.T) {
		m := NewMatchNotifier(sel, testOperatorNS)
		ch := m.Subscribe()
		m.Swap(matching.Name, false) // was non-matching
		stateChanged, isMatching := m.ObserveAndBroadcast(matching)
		assert.True(t, stateChanged)
		assert.True(t, isMatching)
		require.Len(t, ch, 1)
		assert.Equal(t, matching, (<-ch).Object)
	})
}
