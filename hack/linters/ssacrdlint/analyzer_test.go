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

	want := map[int]struct{}{
		29:  {}, // FlaggedUpdate
		84:  {}, // FlaggedMultilineNolintWrongLine (nolint on arg line, not call line)
		104: {}, // FlaggedTrailingNolintPreviousStatement (nolint trails the prior statement)
		111: {}, // FlaggedMethodExprUpdate (method expression, receiver is explicit first arg)
		118: {}, // FlaggedNolintUnrelated (//nolintlint must not suppress this linter)
		126: {}, // FlaggedAliasUpdate (type alias unwrapped via types.Unalias)
		135: {}, // FlaggedPtrAliasUpdate (alias-to-pointer chain requires iterative unwrap)
		144: {}, // FlaggedConversionUpdate (interface conversion hides concrete CRD type)
		170: {}, // FlaggedConcreteConversionUpdate (concrete-target conversion yields CRD type directly)
		178: {}, // FlaggedParenConversionUpdate (parenthesized interface conversion)
		188: {}, // FlaggedBraceNolintPreviousLine (trailing nolint after opening brace)
	}
	const wantMsg = "client.Writer.Update() on an ECK CRD"
	seen := make(map[int]int, len(want))
	for _, d := range diags {
		seen[d.line]++
		if _, exists := want[d.line]; !exists {
			t.Errorf("unexpected diagnostic at line %d: %s", d.line, d.message)
			continue
		}
		if !strings.Contains(d.message, wantMsg) {
			t.Errorf("line %d: want message containing %q, got %q", d.line, wantMsg, d.message)
		}
	}
	for line := range want {
		switch seen[line] {
		case 0:
			t.Errorf("missing diagnostic at line %d", line)
		case 1:
			// expected
		default:
			t.Errorf("line %d: got %d diagnostics, want exactly 1", line, seen[line])
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
