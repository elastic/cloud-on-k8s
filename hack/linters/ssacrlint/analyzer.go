// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package ssacrlint provides a go/analysis linter. It flags direct calls to
// controller-runtime's client.Writer.Update() and client.Writer.Patch() on ECK
// CRs. These calls can cause SSA field-ownership conflicts.
//
// The analyzer reads the object-argument index from client.Writer's own
// interface definition. It uses the parameter whose type is client.Object. The
// check stays correct if the signature gains more leading parameters.
//
// The analyzer identifies a call by the type of its receiver. The receiver must
// implement client.Writer. This excludes SubResourceWriter.Update, which takes a
// different opts type. It also excludes client-go typed clients and every
// unrelated Update method.
//
// # Type tracking
//
// The analyzer uses SSA (Static Single Assignment) form. SSA resolves the
// concrete type of an interface-typed argument at each call site.
//
//   - A [*ssa.MakeInterface] instruction shows the concrete type that goes into
//     the interface. This covers explicit conversions such as
//     client.Object(cr), parenthesized forms, and ordinary assignments.
//   - A [*ssa.Phi] node joins values from conditional branches. The analyzer
//     classifies every edge. All edges must agree for a definitive verdict. If
//     the edges disagree, the analyzer treats the call as unresolved.
//   - For some arguments the analyzer cannot determine a concrete type.
//     Function parameters, cross-function flows, and values that other
//     functions return are examples. The analyzer reports these with a separate
//     "cannot resolve" diagnostic.
//
// The analyzer does not unwind cross-function flows. An ECK CR can pass
// through a client.Object parameter into a helper that calls Update. That site
// produces a diagnostic. Suppress it with //nolint:ssacrlint if the write is
// intentional.
//
// # Suppression
//
// Add a nolint directive to the call. Use it for legitimate full-object writes,
// such as spec changes and data reconciliation.
//
// These forms all work: //nolint:ssacrlint, //nolint:govet,ssacrlint,
// //nolint:all, and bare //nolint.
//
// Put the directive on the same line as the call. You can also put it on the
// line immediately before the call, as a standalone comment. For a multi-line
// call, put it on the line that opens the call. A directive on an argument line
// has no effect.
package ssacrlint

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/ssa"
)

const (
	// clientPkg is the canonical import path for the controller-runtime client package.
	clientPkg = "sigs.k8s.io/controller-runtime/pkg/client"
	// DefaultCRPathPattern is the default regexp matched against package paths to
	// identify ECK CRD types. It matches pkg/apis/ packages under the ECK module
	// root, including the major-version variant (v2/pkg/apis/…). It is also the
	// default value of the -cr-path-pattern flag, which accepts any valid regexp
	// so the pattern can be overridden directly — for example in tests.
	DefaultCRPathPattern = `^github\.com/elastic/cloud-on-k8s/(v\d+/)?pkg/apis/`
)

// config holds per-analyzer-instance configuration. Each instance returned by
// NewAnalyzer has its own config, so tests can create isolated instances without
// mutating shared package-level state.
type config struct {
	crPathPattern string
	once          sync.Once
	crPathRE      *regexp.Regexp // compiled once on the first run() call; read-only thereafter
	crPathREErr   error          // compilation error captured by once.Do; checked at the start of run()
}

// crState classifies how confidently the analyzer can determine whether a
// client.Writer method argument is an ECK CR.
type crState int

const (
	crStateUnknown crState = iota // concrete type could not be determined
	crStateCR                     // all reachable concrete types are ECK CRs
	crStateNonCR                  // all reachable concrete types are non-CRs
	crStateMixed                  // mix of ECK CRs and non-CRs (e.g. conditional branches)
	// crStateInProgress marks a value whose classification is still being
	// computed further up the recursion. It exists only to break cycles and is
	// never returned to callers outside classifySSAValue.
	crStateInProgress
)

// NewAnalyzer returns an ssacrlint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	cfg := &config{crPathPattern: DefaultCRPathPattern}
	a := &analysis.Analyzer{
		Name:     "ssacrlint",
		Doc:      "flags client.Writer.Update() and Patch() calls on ECK CRs that may cause SSA field-ownership conflicts",
		Requires: []*analysis.Analyzer{inspect.Analyzer, buildssa.Analyzer},
		Run:      cfg.run,
	}
	a.Flags.StringVar(&cfg.crPathPattern, "cr-path-pattern", DefaultCRPathPattern,
		"regexp matched against package paths to identify ECK CRD types (default matches pkg/apis/ under the ECK module root)")
	return a
}

func (c *config) run(pass *analysis.Pass) (any, error) {
	c.once.Do(func() {
		c.crPathRE, c.crPathREErr = regexp.Compile(c.crPathPattern)
	})
	if c.crPathREErr != nil {
		return nil, errors.New("invalid -cr-path-pattern: " + c.crPathREErr.Error())
	}
	crPathRE := c.crPathRE

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
	ctrlRuntimePkg := clientWriterNamed.Obj().Pkg()
	clientObjLookup := ctrlRuntimePkg.Scope().Lookup("Object")
	if clientObjLookup == nil {
		return nil, errors.New("client.Object not found in controller-runtime client package - controller-runtime changed its interface")
	}
	clientObjType := clientObjLookup.Type()

	// Derive the object-argument index for each monitored method from
	// client.Writer's interface definition: the parameter whose type is client.Object.
	objIdxByMethod := make(map[string]int, 2)
	for _, methodName := range []string{"Update", "Patch"} {
		idx := methodObjParamIdx(clientWriterIface, methodName, clientObjType)
		if idx < 0 {
			return nil, errors.New("client.Writer." + methodName + " with a client.Object parameter not found - controller-runtime changed its interface")
		}
		objIdxByMethod[methodName] = idx
	}

	// Build an SSA-based map from call-site position to argument CR state.
	// The map covers all invoke-mode calls (regular interface method calls);
	// static method-expression calls fall back to the static type check below.
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	ssaArgStates := computeSSAArgStates(ssaResult.SrcFuncs, objIdxByMethod, clientWriterIface, crPathRE)

	// Build per-file code-position maps once so that hasNolint can determine
	// whether a preceding-line comment stands alone without a full AST walk
	// per call site.
	lineCodeMaps := buildLineCodeMaps(pass.Files, pass.Fset)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}

		objIdx, ok := objIdxByMethod[sel.Sel.Name]
		if !ok {
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

		// For method-expression calls (client.Writer.Update(w, ctx, cr)) sel.X
		// is a type rather than a value, and the receiver is passed as
		// call.Args[0], shifting every parameter index by one.
		actualObjIdx := objIdx
		if tv, ok := pass.TypesInfo.Types[sel.X]; ok && tv.IsType() {
			actualObjIdx = objIdx + 1
		}
		if len(call.Args) <= actualObjIdx {
			return
		}

		// Determine the CR state of the object argument.
		//
		// For invoke-mode calls the SSA map covers interface conversions,
		// parenthesised expressions, MakeInterface chains, and Phi joins —
		// all cases the static type alone cannot resolve.
		// For method-expression (static) calls the SSA map has no entry;
		// those fall back to the static type of the argument.
		argType := pass.TypesInfo.TypeOf(call.Args[actualObjIdx])
		var state crState
		if isECKCR(argType, crPathRE) {
			// Concrete ECK CR — no SSA lookup needed.
			state = crStateCR
		} else if _, isIface := argType.Underlying().(*types.Interface); !isIface {
			// Concrete non-CR type — definitely safe.
			return
		} else {
			// Interface-typed: consult the SSA map.
			// SSA positions call instructions at Lparen; that is the key used
			// in computeSSAArgStates, so Lparen is the only lookup needed.
			if s, ok := ssaArgStates[call.Lparen]; ok {
				state = s
			}
			// state remains crStateUnknown if no entry was found.
		}

		if state == crStateNonCR {
			return
		}

		if hasNolint(pass, call.Pos(), lineCodeMaps) {
			return
		}

		var msg string
		switch state {
		case crStateCR:
			msg = "client.Writer." + sel.Sel.Name + "() on an ECK CR may cause SSA field-ownership conflicts. " +
				"Add //nolint:ssacrlint to mark this as safe and dismiss this report."
		case crStateMixed:
			msg = "client.Writer." + sel.Sel.Name + "() has an interface-typed argument that is an ECK CR on at least one branch. " +
				"This call may cause SSA field-ownership conflicts depending on which branch runs. " +
				"Add //nolint:ssacrlint to mark this as safe and dismiss this report."
		default:
			msg = "client.Writer." + sel.Sel.Name + "() has an interface-typed argument. " +
				"The analyzer cannot resolve its concrete type. " +
				"If the value is an ECK CR, this could cause SSA field-ownership conflicts. " +
				"Add //nolint:ssacrlint to mark this as safe and dismiss this report."
		}
		pass.Report(analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg})
	})

	return nil, nil
}

// computeSSAArgStates iterates all SSA source functions and returns a map from
// call-site position (the Lparen of the call expression in SSA) to the CR
// state of the object argument for every invoke-mode call to a client.Writer
// Update or Patch method.
func computeSSAArgStates(fns []*ssa.Function, objIdxByMethod map[string]int, clientWriterIface *types.Interface, crPathRE *regexp.Regexp) map[token.Pos]crState {
	states := make(map[token.Pos]crState)
	for _, fn := range fns {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				call, ok := instr.(*ssa.Call)
				if !ok || !call.Call.IsInvoke() {
					continue
				}
				method := call.Call.Method
				objIdx, ok := objIdxByMethod[method.Name()]
				if !ok {
					continue
				}
				if !implementsWriter(call.Call.Value.Type(), clientWriterIface) {
					continue
				}
				if objIdx >= len(call.Call.Args) {
					continue
				}
				pos := call.Pos()
				if pos == token.NoPos {
					continue
				}
				states[pos] = classifySSAValue(call.Call.Args[objIdx], make(map[ssa.Value]crState), crPathRE)
			}
		}
	}
	return states
}

// classifySSAValue determines the CR state of an SSA value by inspecting its
// provenance:
//
//   - If the value's static type is already a concrete ECK CR, it returns
//     crStateCR without further analysis (handles ChangeType / concrete
//     conversions whose result type is an ECK CR).
//   - If the value's type is a concrete non-interface non-CR type, it returns
//     crStateNonCR.
//   - For interface-typed values, it looks through [*ssa.MakeInterface]
//     (including nested chains for any(client.Object(cr))) and recursively
//     classifies [*ssa.Phi] operands. A Phi is crStateCR only if every
//     incoming edge is, crStateNonCR only if all are, and crStateMixed
//     otherwise.
//   - All other interface-typed values (parameters, loads, function returns)
//     yield crStateUnknown.
//
// seen caches the state of every value already classified. Caching the result,
// rather than just recording that a value was visited, is required for
// correctness: a value can be reachable from more than one edge of the same Phi
// (nested conditionals reassigning the same variable), and returning
// crStateUnknown on every visit after the first would collapse an otherwise
// unanimous Phi to crStateMixed.
func classifySSAValue(v ssa.Value, seen map[ssa.Value]crState, crPathRE *regexp.Regexp) crState {
	if s, ok := seen[v]; ok {
		return s
	}
	// Seed with the in-progress marker so a value that reaches itself (the
	// back-edge of a loop-carried Phi) terminates instead of recursing forever.
	seen[v] = crStateInProgress
	s := classifySSAValueUncached(v, seen, crPathRE)
	seen[v] = s
	return s
}

// classifySSAValueUncached computes the state of v. Call classifySSAValue
// instead: it adds the caching and cycle handling this function relies on.
func classifySSAValueUncached(v ssa.Value, seen map[ssa.Value]crState, crPathRE *regexp.Regexp) crState {
	// Concrete-type shortcut: works for *ssa.Alloc, *ssa.ChangeType, etc.
	if isECKCR(v.Type(), crPathRE) {
		return crStateCR
	}
	if _, isIface := v.Type().Underlying().(*types.Interface); !isIface {
		return crStateNonCR
	}

	// Interface-typed: look through the value's definition.
	switch v := v.(type) {
	case *ssa.MakeInterface:
		// Unwrap nested interface conversions (e.g. any(client.Object(cr))).
		if _, innerIsIface := v.X.Type().Underlying().(*types.Interface); innerIsIface {
			if s := classifySSAValue(v.X, seen, crPathRE); s != crStateInProgress {
				return s
			}
			return crStateUnknown
		}
		if isECKCR(v.X.Type(), crPathRE) {
			return crStateCR
		}
		return crStateNonCR
	case *ssa.Phi:
		var (
			result      crState
			initialized bool
		)
		for _, edge := range v.Edges {
			s := classifySSAValue(edge, seen, crPathRE)
			// A back-edge into a Phi still being classified contributes nothing:
			// the verdict is decided by the edges that do terminate.
			if s == crStateInProgress {
				continue
			}
			if !initialized {
				result = s
				initialized = true
				continue
			}
			if result == s {
				continue
			}
			// Disagreement between edges. crStateMixed means "at least one
			// branch is definitely a CR", so only return it when one of the
			// sides is crStateCR. Unknown vs NonCR stays crStateUnknown:
			// we cannot confirm any branch is a CR, so the less alarming
			// "cannot resolve" message is more accurate.
			if result == crStateCR || s == crStateCR {
				return crStateMixed
			}
			result = crStateUnknown
		}
		if !initialized {
			return crStateUnknown
		}
		return result
	}
	return crStateUnknown
}

// methodObjParamIdx returns the index of the client.Object parameter in the
// named method's signature as declared in iface, derived from the interface
// definition itself. Returns -1 if the method is not found or has no
// client.Object parameter.
func methodObjParamIdx(iface *types.Interface, methodName string, clientObjType types.Type) int {
	for m := range iface.Methods() {
		if m.Name() != methodName {
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
		// A package reached transitively can be an incomplete stub carrying
		// just its path, with an empty scope. That means the type is simply
		// unavailable here, not that controller-runtime changed: such a
		// package cannot contain a client.Writer call to check anyway, since
		// analyzing one requires the real type information.
		if !pkg.Complete() {
			return nil, nil
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

// isECKCR returns true when t, after iteratively unwrapping any combination
// of type aliases and pointer indirections, is a named type whose package path
// matches crPathRE. The loop is needed because an alias may resolve to a
// pointer (type CR = *esv1.Elasticsearch), and a pointer may wrap an alias —
// so a single pass of each is insufficient.
func isECKCR(t types.Type, crPathRE *regexp.Regexp) bool {
	for {
		t = types.Unalias(t)
		if ptr, ok := t.(*types.Pointer); ok {
			t = ptr.Elem()
			continue
		}
		break
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return false
	}
	return crPathRE.MatchString(pkg.Path())
}

// buildLineCodeMaps returns a map from *token.File to a per-line map of the
// minimum code position (start or end of any non-comment AST node) on each
// line. Used by hasNolint to determine in O(1) whether a comment is standalone
// on its line or trails a prior statement, without a full AST walk per call.
func buildLineCodeMaps(files []*ast.File, fset *token.FileSet) map[*token.File]map[int]token.Pos {
	result := make(map[*token.File]map[int]token.Pos, len(files))
	for _, f := range files {
		tf := fset.File(f.Pos())
		m := make(map[int]token.Pos)
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			addPos := func(p token.Pos) {
				if !p.IsValid() {
					return
				}
				l := fset.Position(p).Line
				if prev, ok := m[l]; !ok || p < prev {
					m[l] = p
				}
			}
			addPos(n.Pos())
			addPos(n.End())
			return true
		})
		result[tf] = m
	}
	return result
}

// hasNolint returns true when the call site's source line, or the immediately
// preceding line, contains a nolint directive that covers this linter:
// //nolint:ssacrlint, //nolint:govet,ssacrlint (combined), //nolint:all,
// or bare //nolint (suppress-all). For the preceding-line case the comment
// must be standalone (no code before it on the same line) so that a trailing
// directive on a prior statement is not misattributed to the Update call.
//
// This function is required in both standalone (go vet -vettool, cmd/main.go)
// and golangci-lint plugin modes. In plugin mode, golangci-lint does apply its
// own nolint post-processing, but only to diagnostics that are actually
// reported via pass.Report — this function suppresses before reporting. More
// importantly, golangci-lint's own processor handles same-line //nolint reliably
// but does not guarantee support for the preceding-line standalone form, which
// is another established suppression pattern. Removing this function would silently
// break that form in plugin mode and remove all suppression in standalone mode.
func hasNolint(pass *analysis.Pass, pos token.Pos, lineCodeMaps map[*token.File]map[int]token.Pos) bool {
	line := pass.Fset.Position(pos).Line
	targetFile := pass.Fset.File(pos)
	lcm := lineCodeMaps[targetFile]
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) != targetFile {
			continue
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				commentLine := pass.Fset.Position(c.Pos()).Line
				if commentLine == line && isNolintSuppressed(c.Text) {
					return true
				}
				if commentLine == line-1 && isNolintSuppressed(c.Text) &&
					!hasCodeBeforeOnLine(lcm, c.Pos(), commentLine) {
					return true
				}
			}
		}
		break
	}
	return false
}

// hasCodeBeforeOnLine reports whether any non-comment AST node starts or ends
// on the given line at a position before commentPos. It uses a precomputed map
// of minimum code positions per line (built once per file by buildLineCodeMaps)
// to answer in O(1). A true result means the comment is trailing on a prior
// statement rather than standing alone on its own line.
func hasCodeBeforeOnLine(lcm map[int]token.Pos, commentPos token.Pos, line int) bool {
	minPos, ok := lcm[line]
	return ok && minPos < commentPos
}

// isNolintSuppressed reports whether text is a nolint directive that covers
// ssacrlint: the specific linter name (alone or in a comma-separated list),
// //nolint:all, or bare //nolint. Linter names are matched as exact tokens to
// avoid false positives from names like "ssacrlintx" or "alliance".
func isNolintSuppressed(text string) bool {
	after, ok := strings.CutPrefix(strings.TrimSpace(text), "//nolint")
	if !ok {
		return false
	}
	// bare //nolint (no linter list, or whitespace-delimited) is suppress-all;
	// any other non-colon suffix (e.g., //nolintlint) is a different directive.
	if after == "" || after[0] == ' ' || after[0] == '\t' {
		return true
	}
	if after[0] != ':' {
		return false
	}
	// parse comma-separated linter names; trim leading/trailing spaces to
	// tolerate //nolint: ssacrlint (space after colon), then cut at the first
	// remaining space/tab to drop trailing inline comments (// reason).
	list := strings.TrimSpace(after[1:])
	if idx := strings.IndexAny(list, " \t"); idx >= 0 {
		list = list[:idx]
	}
	for name := range strings.SplitSeq(list, ",") {
		n := strings.TrimSpace(name)
		if n == "ssacrlint" || n == "all" {
			return true
		}
	}
	return false
}
