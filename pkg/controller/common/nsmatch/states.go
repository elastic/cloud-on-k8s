// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import "sync"

// NamespaceStates tracks the last-known match result for each namespace so the
// flip-state controller can detect state changes without re-evaluating the
// selector on every reconcile loop.
type NamespaceStates struct {
	mu    sync.Mutex
	state map[string]bool // namespace name -> last known match result
}

// NewNamespaceStates returns an initialised NamespaceStates.
func NewNamespaceStates() *NamespaceStates {
	return &NamespaceStates{state: map[string]bool{}}
}

// Swap records isMatching for ns and returns the previously recorded value
// (wasMatching) and whether ns was already known. On the first call for a
// given ns, known is false.
func (n *NamespaceStates) Swap(ns string, isMatching bool) (wasMatching, known bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	wasMatching, known = n.state[ns]
	n.state[ns] = isMatching
	return wasMatching, known
}

// Forget removes the recorded state for ns, typically when a namespace is
// deleted.
func (n *NamespaceStates) Forget(ns string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	delete(n.state, ns)
}
