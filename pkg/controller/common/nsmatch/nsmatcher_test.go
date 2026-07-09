// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

func TestNamespaceMatcherSelectorEnabled(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("nil receiver: disabled", func(t *testing.T) {
		var m *NamespaceMatcher
		assert.False(t, m.SelectorEnabled())
	})

	t.Run("nil selector: disabled", func(t *testing.T) {
		m := NewNamespaceMatcher(nil, testOperatorNS)
		assert.False(t, m.SelectorEnabled())
	})

	t.Run("selector set: enabled", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		assert.True(t, m.SelectorEnabled())
	})
}

func TestNamespaceMatcherNamespaceNameMatches(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("selector disabled: always matches without consulting the cache", func(t *testing.T) {
		m := NewNamespaceMatcher(nil, testOperatorNS)
		matches, err := m.NamespaceNameMatches(t.Context(), "any-ns")
		require.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("empty namespace: always matches without consulting the cache", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(cachemock.NewCache(t)) // no expectations set: a Get call would fail the test
		matches, err := m.NamespaceNameMatches(t.Context(), "")
		require.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("operator namespace: always matches without consulting the cache", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(cachemock.NewCache(t)) // no expectations set: a Get call would fail the test
		matches, err := m.NamespaceNameMatches(t.Context(), testOperatorNS)
		require.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("cache error: error is propagated, no match", func(t *testing.T) {
		cacheErr := errors.New("cache unavailable")
		mc := cachemock.NewCache(t)
		mc.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(cacheErr)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		matches, err := m.NamespaceNameMatches(t.Context(), "dev-ns")
		require.ErrorIs(t, err, cacheErr)
		assert.False(t, matches)
	})

	t.Run("namespace labels satisfy the selector: matches", func(t *testing.T) {
		mc := cachemock.NewCache(t)
		mc.OnGetSetNamespace(map[string]string{"env": "prod"}).Return(nil)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		matches, err := m.NamespaceNameMatches(t.Context(), "prod-ns")
		require.NoError(t, err)
		assert.True(t, matches)
	})

	t.Run("namespace labels do not satisfy the selector: does not match", func(t *testing.T) {
		mc := cachemock.NewCache(t)
		mc.OnGetSetNamespace(map[string]string{"env": "dev"}).Return(nil)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		matches, err := m.NamespaceNameMatches(t.Context(), "dev-ns")
		require.NoError(t, err)
		assert.False(t, matches)
	})
}

func TestNamespaceMatcherNamespaceMatches(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("selector disabled: always matches", func(t *testing.T) {
		m := NewNamespaceMatcher(nil, testOperatorNS)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev-ns"}}
		assert.True(t, m.NamespaceMatches(ns))
	})

	t.Run("always-managed namespace: matches regardless of labels", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testOperatorNS}}
		assert.True(t, m.NamespaceMatches(ns))
	})

	t.Run("labels satisfy the selector: matches", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-ns", Labels: map[string]string{"env": "prod"}}}
		assert.True(t, m.NamespaceMatches(ns))
	})

	t.Run("labels do not satisfy the selector: does not match", func(t *testing.T) {
		m := NewNamespaceMatcher(sel, testOperatorNS)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev-ns", Labels: map[string]string{"env": "dev"}}}
		assert.False(t, m.NamespaceMatches(ns))
	})
}

func TestNamespaceMatcherMatchingNamespaces(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("selector disabled: no namespaces, cache not consulted", func(t *testing.T) {
		m := NewNamespaceMatcher(nil, testOperatorNS)
		m.SetCache(cachemock.NewCache(t)) // no expectations set: a List call would fail the test
		names, err := m.MatchingNamespaces(t.Context())
		require.NoError(t, err)
		assert.Nil(t, names)
	})

	t.Run("cache error: error is wrapped and returned", func(t *testing.T) {
		listErr := errors.New("api server unavailable")
		mc := cachemock.NewCache(t)
		mc.OnListSetNamespaceList().Return(listErr)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		names, err := m.MatchingNamespaces(t.Context())
		require.ErrorIs(t, err, listErr)
		assert.Nil(t, names)
	})

	t.Run("returns matching namespaces plus always-managed ones, deduplicated", func(t *testing.T) {
		mc := cachemock.NewCache(t)
		mc.OnListSetNamespaceList(
			corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-ns", Labels: map[string]string{"env": "prod"}}},
			corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev-ns", Labels: map[string]string{"env": "dev"}}},
			corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testOperatorNS}},
		).Return(nil)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		names, err := m.MatchingNamespaces(t.Context())
		require.NoError(t, err)
		// prod-ns matches the selector, dev-ns does not, and testOperatorNS is always
		// included even though it's also present (once) in the listed namespaces.
		assert.ElementsMatch(t, []string{"prod-ns", testOperatorNS}, names)
	})

	t.Run("always-managed namespace is included even when absent from the listed namespaces", func(t *testing.T) {
		mc := cachemock.NewCache(t)
		mc.OnListSetNamespaceList(
			corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dev-ns", Labels: map[string]string{"env": "dev"}}},
		).Return(nil)

		m := NewNamespaceMatcher(sel, testOperatorNS)
		m.SetCache(mc)
		names, err := m.MatchingNamespaces(t.Context())
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{testOperatorNS}, names)
	})
}
