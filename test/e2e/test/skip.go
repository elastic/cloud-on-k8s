// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import "testing"

// SkipIfStateless skips the test if running in stateless Elasticsearch mode.
// Use this for tests that are only valid for stateful Elasticsearch deployments.
func SkipIfStateless(t *testing.T, reason string) {
	t.Helper()
	if Ctx().IsStateless() {
		t.Skipf("Test skipped in stateless mode: %s", reason)
	}
}

// SkipIfStateful skips the test if running in stateful Elasticsearch mode.
// Use this for tests that are only valid for stateless Elasticsearch deployments.
func SkipIfStateful(t *testing.T, reason string) {
	t.Helper()
	if !Ctx().IsStateless() {
		t.Skipf("Test skipped in stateful mode: %s", reason)
	}
}

// RequireStateless skips the test if not running in stateless mode.
// Use this for tests that require stateless infrastructure (e.g., object storage).
func RequireStateless(t *testing.T) {
	t.Helper()
	if !Ctx().IsStateless() {
		t.Skip("Test requires stateless mode")
	}
}

// RequireStateful skips the test if not running in stateful mode.
// Use this for tests that require stateful infrastructure (e.g., StatefulSets).
func RequireStateful(t *testing.T) {
	t.Helper()
	if Ctx().IsStateless() {
		t.Skip("Test requires stateful mode")
	}
}
