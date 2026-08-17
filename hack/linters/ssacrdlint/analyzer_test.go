// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package ssacrdlint_test

import (
	"go/types"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrdlint"
)

type resolvedDiag struct {
	line    int
	message string
}

// fakeAPIPrefix is the module-relative package prefix used by the in-module fake
// CRD types in testcases/fakeapi/. The test overrides the analyzer's -crd-module-prefix
// flag to this value so the real ECK module is not required as a dependency.
const fakeAPIPrefix = "github.com/elastic/cloud-on-k8s/hack/linters/ssacrdlint/testcases/fakeapi/"

func thisDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}

func TestAnalyzerDefinition(t *testing.T) {
	if err := analysis.Validate([]*analysis.Analyzer{ssacrdlint.Analyzer}); err != nil {
		t.Fatalf("analyzer definition invalid: %v", err)
	}
}

func TestAnalyzer(t *testing.T) {
	if err := ssacrdlint.Analyzer.Flags.Set("crd-module-prefix", fakeAPIPrefix); err != nil {
		t.Fatalf("set -crd-module-prefix flag: %v", err)
	}
	t.Cleanup(func() {
		_ = ssacrdlint.Analyzer.Flags.Set("crd-module-prefix", ssacrdlint.DefaultCRDModulePrefix)
	})

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir: thisDir(),
	}
	pkgs, err := packages.Load(cfg, "./testcases/")
	if err != nil {
		t.Fatalf("load testcases: %v", err)
	}
	if n := packages.PrintErrors(pkgs); n > 0 {
		t.Fatalf("%d package error(s)", n)
	}

	diags := runAnalyzer(t, pkgs)

	// FlaggedUpdate is at testcases/testcases.go:28,
	// FlaggedMultilineNolintWrongLine at :83 (nolint on arg line, not call line).
	// All NotFlagged* call sites must be clean.
	want := map[int]string{
		28: "Update",
		83: "Update",
	}
	if len(diags) != len(want) {
		t.Errorf("expected %d diagnostics, got %d", len(want), len(diags))
		for _, d := range diags {
			t.Logf("  line %d: %s", d.line, d.message)
		}
		return
	}
	for _, d := range diags {
		method, ok := want[d.line]
		if !ok {
			t.Errorf("unexpected diagnostic at line %d: %s", d.line, d.message)
			continue
		}
		if !strings.Contains(d.message, method) {
			t.Errorf("line %d: want %q in message, got %q", d.line, method, d.message)
		}
	}
}

// runAnalyzer loads the inspect prerequisite manually and runs ssacrdlint.Analyzer
// on the supplied packages, collecting all reported diagnostics with resolved line numbers.
func runAnalyzer(t *testing.T, pkgs []*packages.Package) []resolvedDiag {
	t.Helper()
	var all []resolvedDiag
	for _, pkg := range pkgs {
		insp := inspector.New(pkg.Syntax)
		pass := &analysis.Pass{
			Analyzer:   ssacrdlint.Analyzer,
			Fset:       pkg.Fset,
			Files:      pkg.Syntax,
			Pkg:        pkg.Types,
			TypesInfo:  pkg.TypesInfo,
			TypesSizes: types.SizesFor("gc", runtime.GOARCH),
			ResultOf: map[*analysis.Analyzer]any{
				inspect.Analyzer: insp,
			},
			Report: func(d analysis.Diagnostic) {
				all = append(all, resolvedDiag{
					line:    pkg.Fset.Position(d.Pos).Line,
					message: d.Message,
				})
			},
		}
		if _, err := ssacrdlint.Analyzer.Run(pass); err != nil {
			t.Errorf("analyzer.Run: %v", err)
		}
	}
	return all
}
