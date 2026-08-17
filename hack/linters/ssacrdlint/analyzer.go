// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package ssacrdlint provides a go/analysis linter that flags direct calls
// to controller-runtime's client.Writer.Update() to prevent unintended SSA
// field-ownership conflicts.
//
// The object-argument index is derived from client.Writer's own interface
// definition by finding the parameter whose type is client.Object, so the
// check remains correct if the signature ever gains additional leading
// parameters.
//
// The analyzer identifies calls by checking whether the method's declaring
// interface implements client.Writer. This precisely excludes
// SubResourceWriter.Update (different opts type), client-go typed clients,
// and any unrelated Update method.
//
// Suppression: add a nolint directive on the opening line of the call for
// legitimate full-object writes (spec changes, data reconciliation, etc.).
// Accepted forms: //nolint:ssacrdlint, //nolint:govet,ssacrdlint (combined),
// //nolint:all, or bare //nolint. For multi-line calls the directive must
// appear on the same line as the receiver (c.Update( //nolint:ssacrdlint),
// not on a subsequent argument line.
package ssacrdlint

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const (
	// clientPkg is the canonical import path for the controller-runtime client package.
	clientPkg = "sigs.k8s.io/controller-runtime/pkg/client"
	// DefaultCRDModulePrefix is the ECK module path prefix used to identify ECK CRD types.
	DefaultCRDModulePrefix = "github.com/elastic/cloud-on-k8s/"
)

// crdModulePrefix is the effective module path prefix used to identify ECK CRD types. It defaults
// to DefaultCRDModulePrefix and can be overridden via the -crd-module-prefix flag (useful in
// tests that use an in-module fake CRD package instead of the real ECK types).
var crdModulePrefix = DefaultCRDModulePrefix

// Analyzer is the go/analysis analyzer exported for use in golangci-lint plugins
// and standalone go-vet-style invocations.
var Analyzer = &analysis.Analyzer{
	Name:     "ssacrdlint",
	Doc:      "flags client.Writer.Update() calls on ECK CRDs that may cause SSA field-ownership conflicts",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func init() {
	Analyzer.Flags.StringVar(&crdModulePrefix, "crd-module-prefix", DefaultCRDModulePrefix,
		"module path prefix that identifies ECK CRD types")
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Look up client.Writer from the import graph once per package.
	// If the package doesn't transitively import controller-runtime there is
	// nothing to check.
	clientWriterType, err := lookupType(pass.Pkg, clientPkg, "Writer")
	if err != nil {
		return nil, err
	}
	if clientWriterType == nil {
		return nil, nil
	}
	clientWriterNamed, ok := clientWriterType.(*types.Named)
	if !ok {
		return nil, errors.New("client.Writer is not a named type - controller-runtime changed its interface")
	}
	clientWriterIface, ok := clientWriterNamed.Underlying().(*types.Interface)
	if !ok {
		return nil, errors.New("client.Writer underlying type is not an interface - controller-runtime changed its interface")
	}

	// client.Object is in the same package as client.Writer — look it up directly
	// from the package scope without a second import-graph traversal.
	crPkg := clientWriterNamed.Obj().Pkg()
	clientObjLookup := crPkg.Scope().Lookup("Object")
	if clientObjLookup == nil {
		return nil, errors.New("client.Object not found in controller-runtime client package - controller-runtime changed its interface")
	}
	clientObjType := clientObjLookup.Type()

	// Derive the object-argument index for Update from client.Writer's interface
	// definition: the parameter whose type is client.Object.
	objIdx := updateObjParamIdx(clientWriterIface, clientObjType)
	if objIdx < 0 {
		return nil, errors.New("client.Writer.Update with a client.Object parameter not found - controller-runtime changed its interface")
	}

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}

		if sel.Sel.Name != "Update" {
			return
		}

		obj, ok := pass.TypesInfo.Uses[sel.Sel]
		if !ok {
			return
		}

		fn, ok := obj.(*types.Func)
		if !ok {
			return
		}

		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			return
		}

		recv := sig.Recv()
		if recv == nil {
			return
		}

		// Use types.Implements against the looked-up client.Writer interface.
		// SubResourceWriter.Update uses different opts types so it does not
		// satisfy client.Writer and is automatically excluded.
		if !implementsWriter(recv.Type(), clientWriterIface) {
			return
		}

		// Only flag calls where the object argument is an ECK CRD (a type from
		// pkg/apis/). Calls on plain Kubernetes types (Secret, StatefulSet, …)
		// are not our concern.
		if len(call.Args) <= objIdx || !isECKCRD(pass.TypesInfo.TypeOf(call.Args[objIdx])) {
			return
		}

		if hasNolint(pass, call.Pos()) {
			return
		}

		pass.Reportf(call.Pos(),
			"client.Writer.Update() on an ECK CRD may cause SSA field-ownership conflicts; "+
				"add //nolint:ssacrdlint for legitimate full-object writes")
	})

	return nil, nil
}

// updateObjParamIdx returns the index of the client.Object parameter in
// client.Writer.Update's signature, derived from the interface definition
// itself. Returns -1 if Update is not found or has no matching parameter.
func updateObjParamIdx(iface *types.Interface, clientObjType types.Type) int {
	for m := range iface.Methods() {
		if m.Name() != "Update" {
			continue
		}
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			continue
		}
		for j := 0; j < sig.Params().Len(); j++ {
			if types.Identical(sig.Params().At(j).Type(), clientObjType) {
				return j
			}
		}
	}
	return -1
}

// implementsWriter reports whether t (or *t for non-pointer concrete types)
// satisfies iface.
func implementsWriter(t types.Type, iface *types.Interface) bool {
	if types.Implements(t, iface) {
		return true
	}
	if _, isPtr := t.(*types.Pointer); !isPtr {
		return types.Implements(types.NewPointer(t), iface)
	}
	return false
}

// lookupType traverses the import graph rooted at pkg to find the named type
// typeName in the package identified by pkgPath. Returns (nil, nil) if pkgPath
// is not reachable, (type, nil) on success, or (nil, error) if the package is
// found but typeName is absent — indicating a breaking change in the dependency.
func lookupType(pkg *types.Package, pkgPath, typeName string) (types.Type, error) {
	visited := make(map[*types.Package]bool)
	return searchImports(pkg, pkgPath, typeName, visited)
}

func searchImports(pkg *types.Package, pkgPath, typeName string, visited map[*types.Package]bool) (types.Type, error) {
	if visited[pkg] {
		return nil, nil
	}
	visited[pkg] = true
	if pkg.Path() == pkgPath {
		if obj := pkg.Scope().Lookup(typeName); obj != nil {
			return obj.Type(), nil
		}
		return nil, errors.New(pkgPath + "." + typeName + " not found - controller-runtime changed its interface")
	}
	for _, imp := range pkg.Imports() {
		if t, err := searchImports(imp, pkgPath, typeName, visited); err != nil || t != nil {
			return t, err
		}
	}
	return nil, nil
}

// isECKCRD returns true when t (unwrapping a pointer if needed) is a named
// type whose package path starts with the ECK module prefix.
func isECKCRD(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return false
	}
	return strings.HasPrefix(pkg.Path(), crdModulePrefix)
}

// hasNolint returns true when the call site's source line contains a nolint
// directive that covers this linter: //nolint:ssacrdlint, //nolint:govet,ssacrdlint
// (combined), //nolint:all, or bare //nolint (suppress-all).
func hasNolint(pass *analysis.Pass, pos token.Pos) bool {
	line := pass.Fset.Position(pos).Line
	targetFile := pass.Fset.File(pos)
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) != targetFile {
			continue
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if pass.Fset.Position(c.Pos()).Line == line && isNolintSuppressed(c.Text) {
					return true
				}
			}
		}
		break
	}
	return false
}

// isNolintSuppressed reports whether text is a nolint directive that covers
// ssacrdlint: the specific linter name (alone or in a comma-separated list),
// //nolint:all, or bare //nolint.
func isNolintSuppressed(text string) bool {
	// ":ssacrdlint" matches //nolint:ssacrdlint (first in list).
	// ",ssacrdlint" matches //nolint:govet,ssacrdlint (not first in list).
	if strings.Contains(text, ":ssacrdlint") || strings.Contains(text, ",ssacrdlint") ||
		strings.Contains(text, "nolint:all") {
		return true
	}
	// bare //nolint (no linter list) is a suppress-all directive
	after, ok := strings.CutPrefix(strings.TrimSpace(text), "//nolint")
	return ok && (after == "" || after[0] != ':')
}
