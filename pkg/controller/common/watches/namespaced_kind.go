// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nsmatch"
)

// NamespacedKind is a drop-in replacement for source.Kind that injects a
// namespace-scope predicate based on the supplied NamespaceMatcher. When the
// matcher is disabled (legacy / static-resolution modes) the predicate is a
// no-op, so callers can wire this unconditionally.
//
// Use this for every namespaced resource watch. Cluster-scoped watches (e.g.
// webhook configurations, cluster-scoped license state) should continue to use
// source.Kind directly; the matcher already short-circuits empty namespaces,
// but skipping the cache lookup is the cleaner signal.
func NamespacedKind[T client.Object](
	m *nsmatch.NamespaceMatcher,
	c cache.Cache,
	obj T,
	h handler.TypedEventHandler[T, reconcile.Request],
	preds ...predicate.TypedPredicate[T],
) source.SyncingSource {
	if !m.SelectorEnabled() {
		return source.Kind(c, obj, h, preds...)
	}
	nsPred := predicate.NewTypedPredicateFuncs(func(o T) bool {
		// predicate.Filter has no context parameter to propagate; Matches only
		// uses ctx for the cache read's cancellation, so context.TODO() is safe
		// here (no deadline tied to a reconcile loop applies to this lookup).
		return m.NamespaceNameMatches(context.TODO(), o.GetNamespace())
	})
	all := make([]predicate.TypedPredicate[T], 0, len(preds)+1)
	all = append(all, nsPred)
	all = append(all, preds...)
	return source.Kind(c, obj, h, all...)
}
