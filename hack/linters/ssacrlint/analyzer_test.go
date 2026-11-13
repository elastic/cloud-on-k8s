// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package ssacrlint_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint"
)

// fakeAPIPrefix is the full regexp passed to NewAnalyzer in tests. It matches
// the in-module fake CR types under testcases/fakeapi/pkg/apis/ so that the
// real ECK module is not required as a dependency.
const fakeAPIPrefix = `^github\.com/elastic/cloud-on-k8s/hack/linters/ssacrlint/testcases/fakeapi/(v\d+/)?pkg/apis/`

func thisDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func TestAnalyzerDefinition(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{ssacrlint.NewAnalyzer()}); err != nil {
		t.Fatalf("analyzer definition invalid: %v", err)
	}
}

// TestAnalyzerNoControllerRuntime verifies that the analyzer emits no
// diagnostics and returns without error for a package that does not transitively
// import sigs.k8s.io/controller-runtime/pkg/client. The early-exit path in
// run() relies on lookupType returning (nil, nil) for such packages.
func TestAnalyzerNoControllerRuntime(t *testing.T) {
	analysistest.Run(t, thisDir(), ssacrlint.NewAnalyzer(), "./testcases/noclient/")
}

// TestAnalyzer loads testcases/testcases.go and verifies that the analyzer
// reports exactly the diagnostics annotated with // want "..." in the source.
// Expected diagnostics are annotated inline in testcases/testcases.go with:
//
//	// want "on an ECK CR"             — concrete ECK CR detected
//	// want "cannot resolve its concrete type" — interface-typed argument, type unknown
//
// analysistest fails the test for any diagnostic that has no matching annotation
// and for any annotation that has no matching diagnostic.
func TestAnalyzer(t *testing.T) {
	// Use a fresh analyzer instance so that the shared Analyzer var is not
	// mutated and tests can safely run in parallel.
	a := ssacrlint.NewAnalyzer()
	if err := a.Flags.Set("cr-path-pattern", fakeAPIPrefix); err != nil {
		t.Fatalf("setting cr-path-pattern: %v", err)
	}
	analysistest.Run(t, thisDir(), a, "./testcases/")
}

// TestAnalyzerInvalidPattern verifies that run() returns an error when
// -cr-path-pattern is set to an invalid regular expression.
func TestAnalyzerInvalidPattern(t *testing.T) {
	a := ssacrlint.NewAnalyzer()
	if err := a.Flags.Set("cr-path-pattern", "[invalid"); err != nil {
		t.Fatalf("Flags.Set: %v", err)
	}

	// analysistest reports Result.Err via its Testing.Errorf; use a sink so
	// that the expected error does not fail the outer test. We assert on Err directly.
	results := analysistest.Run(&errSink{}, thisDir(), a, "./testcases/noclient/")
	for _, r := range results {
		if r.Err == nil {
			t.Errorf("package %s: expected error for invalid -cr-path-pattern, got nil", r.Pass.Pkg.Path())
		}
	}
}

// errSink satisfies analysistest.Testing by discarding all Errorf calls.
type errSink struct{}

func (errSink) Errorf(string, ...any) {}
func (errSink) Helper()               {}
