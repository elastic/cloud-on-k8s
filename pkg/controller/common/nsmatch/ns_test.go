// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
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
		assert.False(t, (*MatchNotifier)(nil).SelectorEnabled(), "nil MatchNotifier is disabled")
		assert.False(t, NewMatchNotifier(nil, nil, testOperatorNS).SelectorEnabled(), "nil selector is disabled")
		assert.True(t, NewMatchNotifier(nil, mustSelector(t, map[string]string{"env": "prod"}), testOperatorNS).SelectorEnabled(), "non-nil selector is enabled")
	})

	t.Run("Matches disabled always returns true", func(t *testing.T) {
		m := NewMatchNotifier(nil, nil, testOperatorNS)
		assert.True(t, m.Matches(t.Context(), "any-namespace"), "disabled matcher always matches")
		assert.True(t, m.Matches(t.Context(), ""), "disabled matcher matches empty namespace")
	})

	t.Run("Matches empty namespace always matches", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		// Cache must not be called for empty namespace: pass a mock with no expectations.
		c := newMockCache(t)
		m := NewMatchNotifier(c, sel, testOperatorNS)
		assert.True(t, m.Matches(t.Context(), ""), "cluster-scoped (empty namespace) always matches")
	})

	t.Run("Matches operator namespace always matches without cache lookup", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		// No cache expectations: the operator namespace must be short-circuited.
		c := newMockCache(t)
		m := NewMatchNotifier(c, sel, testOperatorNS)
		assert.True(t, m.Matches(t.Context(), testOperatorNS), "operator namespace always matches")
	})

	t.Run("Matches cache miss returns false", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		c := newMockCache(t)
		c.OnGetNotFound("unknown")
		m := NewMatchNotifier(c, sel, testOperatorNS)
		assert.False(t, m.Matches(t.Context(), "unknown"), "cache miss returns false")
	})

	t.Run("Matches labels match", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		c := newMockCache(t)
		c.OnGetFound(namespace("prod-ns", map[string]string{"env": "prod"}))
		m := NewMatchNotifier(c, sel, testOperatorNS)
		assert.True(t, m.Matches(t.Context(), "prod-ns"))
	})

	t.Run("Matches labels no match", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		c := newMockCache(t)
		c.OnGetFound(namespace("dev-ns", map[string]string{"env": "dev"}))
		m := NewMatchNotifier(c, sel, testOperatorNS)
		assert.False(t, m.Matches(t.Context(), "dev-ns"))
	})

	t.Run("MatchesLabels disabled always returns true", func(t *testing.T) {
		m := NewMatchNotifier(nil, nil, testOperatorNS)
		assert.True(t, m.MatchesLabels(map[string]string{"env": "dev"}), "disabled matcher always matches labels")
		assert.True(t, m.MatchesLabels(nil), "disabled matcher matches nil labels")
	})

	t.Run("MatchesLabels match", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(nil, sel, testOperatorNS)
		assert.True(t, m.MatchesLabels(map[string]string{"env": "prod", "team": "platform"}))
	})

	t.Run("MatchesLabels no match", func(t *testing.T) {
		sel := mustSelector(t, map[string]string{"env": "prod"})
		m := NewMatchNotifier(nil, sel, testOperatorNS)
		assert.False(t, m.MatchesLabels(map[string]string{"env": "staging"}))
	})
}

func TestNotifier(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("no subscribers", func(t *testing.T) {
		n := NewMatchNotifier(nil, sel, "")
		// Broadcast with no subscribers must not panic.
		n.Broadcast(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns"}})
	})

	t.Run("single subscriber receives broadcast", func(t *testing.T) {
		n := NewMatchNotifier(nil, sel, "")
		ch := n.Subscribe()

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}}
		n.Broadcast(ns)

		require.Len(t, ch, 1)
		assert.Equal(t, ns, (<-ch).Object)
	})

	t.Run("multiple subscribers each receive broadcast", func(t *testing.T) {
		n := NewMatchNotifier(nil, sel, "")
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
		n := NewMatchNotifier(nil, sel, "")
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
		n := NewMatchNotifier(nil, sel, "")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-c"}}
		n.Broadcast(ns) // no subscribers yet

		ch := n.Subscribe()
		assert.Len(t, ch, 0, "late subscriber must not receive events broadcast before it subscribed")
	})
}
