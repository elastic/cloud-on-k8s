// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package ssacrlint provides a go/analysis linter. It flags direct calls to
// controller-runtime's client.Writer.Update() and certain client.Writer.Patch()
// calls on ECK CRs that may cause SSA field-ownership conflicts.
//
// For Patch calls, only implicit-ownership patch types are flagged: MergeFrom,
// StrategicMergeFrom, MergeFromWithOptions, and RawPatch with MergePatchType or
// StrategicMergePatchType. Explicit-field patch types — RawPatch(JSONPatchType),
// RawPatch(ApplyPatchType), and client.Apply — are not flagged.
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
//     classifies every edge. If all edges are ECK CRs the call is flagged as a
//     CR write. If at least one edge is a CR (and another is not), the analyzer
//     emits the mixed-branch diagnostic. If no edge is a confirmed CR, the call
//     is treated as unresolved.
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
// When run through golangci-lint, add //nolint:ssacrlint to the call line to
// mark a full-object write as intentional. Suppression is handled by
// golangci-lint's own nolint processing; the analyzer always reports the
// diagnostic. When run as a standalone go vet tool, there is no suppression
// mechanism.
package ssacrlint

import (
	"errors"
	"go/ast"
	goconstant "go/constant"
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
	ktypes "k8s.io/apimachinery/pkg/types"
)

const (
	// clientPkg is the canonical import path for the controller-runtime client package.
	clientPkg = "sigs.k8s.io/controller-runtime/pkg/client"
	// DefaultCRPathPattern is the default regexp matched against package paths to
	// identify ECK CRD types. It matches pkg/apis/ packages under the ECK module
	// root, including major-version layouts (v2/pkg/apis/…, v3/pkg/apis/…, etc.).
	// It is also the default value of the -cr-path-pattern flag, which accepts
	// any valid regexp so the pattern can be overridden — for example in tests or
	// when running the linter on a fork with a different module path.
	DefaultCRPathPattern = `^github\.com/elastic/cloud-on-k8s/(v\d+/)?pkg/apis/`
)

// config holds per-analyzer-instance configuration. Each instance returned by
// NewAnalyzer has its own config, so tests can create isolated instances without
// mutating shared package-level state.
type config struct {
	crPathPattern string
	once          sync.Once      // fires on the first run() call; flags must not change after that
	crPathRE      *regexp.Regexp // compiled once on the first run() call; read-only thereafter
	crPathREErr   error          // compilation error captured by once.Do; checked at the start of run()
}

// patchSafety classifies whether the patch argument of a client.Writer.Patch()
// call uses an explicit-field patch type (safe) or an implicit-ownership patch
// type (unsafe).
type patchSafety int

const (
	patchSafetyUnknown    patchSafety = iota // patch type could not be determined; flag conservatively
	patchSafetySafe                          // explicit-field patch (JSONPatch, Apply): do not flag
	patchSafetyUnsafe                        // implicit-ownership patch (MergeFrom, StrategicMergeFrom): flag
	patchSafetyInProgress                    // cycle-breaking sentinel for Phi traversal; never returned to callers
)

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
		Doc:      "flags client.Writer.Update() and implicit-ownership Patch() calls on ECK CRs that may cause SSA field-ownership conflicts",
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

	// client.Object and client.Patch are in the same package as client.Writer —
	// look them up directly from the package scope without a second import-graph traversal.
	ctrlRuntimePkg := clientWriterNamed.Obj().Pkg()
	clientObjLookup := ctrlRuntimePkg.Scope().Lookup("Object")
	if clientObjLookup == nil {
		return nil, errors.New("client.Object not found in controller-runtime client package - controller-runtime changed its interface")
	}
	clientObjType := clientObjLookup.Type()

	clientPatchLookup := ctrlRuntimePkg.Scope().Lookup("Patch")
	if clientPatchLookup == nil {
		return nil, errors.New("client.Patch not found in controller-runtime client package - controller-runtime changed its interface")
	}
	clientPatchType := clientPatchLookup.Type()

	// Derive the object-argument index for each monitored method from
	// client.Writer's interface definition: the parameter whose type is client.Object.
	objIdxByMethod := make(map[string]int, 2)
	for _, methodName := range []string{"Update", "Patch"} {
		idx := methodParamIdxByType(clientWriterIface, methodName, clientObjType)
		if idx < 0 {
			return nil, errors.New("client.Writer." + methodName + " with a client.Object parameter not found - controller-runtime changed its interface")
		}
		objIdxByMethod[methodName] = idx
	}

	// Derive the patch-argument index for client.Writer.Patch from the interface
	// definition: the parameter whose type is client.Patch.
	patchPatchIdx := methodParamIdxByType(clientWriterIface, "Patch", clientPatchType)
	if patchPatchIdx < 0 {
		return nil, errors.New("client.Writer.Patch with a client.Patch parameter not found - controller-runtime changed its interface")
	}

	// Build SSA-based maps from call-site position to CR state and patch safety
	// in a single pass over all source functions.
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	ssaArgStates, ssaPatchSafeties := computeSSACallData(ssaResult.SrcFuncs, objIdxByMethod, patchPatchIdx, clientWriterIface, clientPkg, crPathRE)

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
		isMethodExpr := false
		if tv, ok := pass.TypesInfo.Types[sel.X]; ok && tv.IsType() {
			isMethodExpr = true
		}
		actualObjIdx := objIdx
		if isMethodExpr {
			actualObjIdx = objIdx + 1
		}
		if len(call.Args) <= actualObjIdx {
			return
		}

		// For Patch calls, skip if the patch type is structurally safe (explicit
		// fields only: JSONPatch, ApplyPatch, client.Apply). Only implicit-ownership
		// patch types (MergeFrom, StrategicMergeFrom, unknown) proceed to the CR check.
		if sel.Sel.Name == "Patch" {
			actualPatchIdx := patchPatchIdx
			if isMethodExpr {
				actualPatchIdx = patchPatchIdx + 1
			}
			// If the patch argument is present and its type is safe, skip the CR check.
			// If it is absent (malformed call) or its safety is unknown/unsafe,
			// fall through so the CR check runs conservatively.
			if len(call.Args) > actualPatchIdx {
				if safety, ok := ssaPatchSafeties[call.Lparen]; ok && safety == patchSafetySafe {
					return
				}
			}
		}

		// Determine the CR state of the object argument.
		//
		// The SSA map covers both invoke-mode calls and static
		// method-expression calls. For interface-typed arguments the map
		// resolves MakeInterface chains, parenthesised expressions, and
		// Phi joins — all cases the static type alone cannot resolve.
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

		// For Patch calls, direct developers to safe alternatives. For Update, nolint
		// is the only option since a full-object replace has no targeted equivalent.
		nolintSuffix := "Add //nolint:ssacrlint to mark this as safe and dismiss this report."
		if sel.Sel.Name == "Patch" {
			nolintSuffix = "Use a scoped JSON Patch or client.Apply, or add //nolint:ssacrlint if this full-object write is intentional."
		}

		var msg string
		switch state {
		case crStateCR:
			msg = "client.Writer." + sel.Sel.Name + "() on an ECK CR may cause SSA field-ownership conflicts. " + nolintSuffix
		case crStateMixed:
			msg = "client.Writer." + sel.Sel.Name + "() has an interface-typed argument that is an ECK CR on at least one branch. " +
				"This call may cause SSA field-ownership conflicts depending on which branch runs. " + nolintSuffix
		default:
			msg = "client.Writer." + sel.Sel.Name + "() has an interface-typed argument. " +
				"The analyzer cannot resolve its concrete type. " +
				"If the value is an ECK CR, this could cause SSA field-ownership conflicts. " + nolintSuffix
		}
		pass.Report(analysis.Diagnostic{Pos: call.Pos(), End: call.End(), Message: msg})
	})

	return nil, nil
}

// normalizeSSAMethodName recovers the bare method name from an SSA callee name.
// SSA names interface method-expression wrappers as "<Method>$thunk"
// (e.g. "Update$thunk") and may prefix a receiver type (e.g. "(T).Update").
// This function strips both to produce the plain method name ("Update").
func normalizeSSAMethodName(name string) string {
	if i := strings.IndexByte(name, '$'); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// computeSSACallData performs a single pass over all SSA source functions and
// returns two maps keyed by call-site position (the Lparen SSA assigns to each
// call instruction):
//   - argStates: the CR state of the client.Object argument for every
//     client.Writer Update and Patch call (invoke-mode and method-expression).
//   - patchSafeties: the patchSafety of the client.Patch argument for every
//     client.Writer Patch call.
func computeSSACallData(
	fns []*ssa.Function,
	objIdxByMethod map[string]int,
	patchPatchIdx int,
	clientWriterIface *types.Interface,
	clientPkgPath string,
	crPathRE *regexp.Regexp,
) (argStates map[token.Pos]crState, patchSafeties map[token.Pos]patchSafety) {
	argStates = make(map[token.Pos]crState)
	patchSafeties = make(map[token.Pos]patchSafety)
	for _, fn := range fns {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				var common *ssa.CallCommon
				switch typed := instr.(type) {
				case *ssa.Call:
					common = &typed.Call
				case *ssa.Go:
					common = &typed.Call
				case *ssa.Defer:
					common = &typed.Call
				default:
					continue
				}
				// Use CallCommon.Pos() (the call's Lparen) rather than the
				// instruction's own Pos(), which for *ssa.Go/*ssa.Defer is the
				// go/defer keyword — a different position than what the AST pass
				// uses as its map key (ast.CallExpr.Lparen).
				pos := common.Pos()
				if pos == token.NoPos {
					continue
				}

				// Resolve the method name and the argument-index shift: 0 for
				// invoke-mode calls (c.Update(ctx, obj)), 1 for method-expression
				// calls (client.Client.Update(c, ctx, obj)) where the receiver is
				// passed explicitly as Args[0] and method params start at Args[1].
				var methodName string
				var shift int
				if common.IsInvoke() {
					if !implementsWriter(common.Value.Type(), clientWriterIface) {
						continue
					}
					methodName = common.Method.Name()
				} else {
					callee := common.StaticCallee()
					if callee == nil || len(common.Args) == 0 {
						continue
					}
					if !implementsWriter(common.Args[0].Type(), clientWriterIface) {
						continue
					}
					methodName = normalizeSSAMethodName(callee.Name())
					shift = 1
				}

				// Classify the client.Object argument for Update and Patch calls.
				if objIdx, ok := objIdxByMethod[methodName]; ok {
					if argIdx := objIdx + shift; argIdx < len(common.Args) {
						argStates[pos] = classifySSAValue(common.Args[argIdx], make(map[ssa.Value]crState), crPathRE)
					}
				}

				// Classify the client.Patch argument for Patch calls.
				if methodName == "Patch" {
					if patchArgIdx := patchPatchIdx + shift; patchArgIdx < len(common.Args) {
						patchSafeties[pos] = classifyPatchSSAValue(common.Args[patchArgIdx], make(map[ssa.Value]patchSafety), clientPkgPath)
					}
				}
			}
		}
	}
	return argStates, patchSafeties
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
			// branch is definitely a CR". Preserve it whenever either
			// operand is crStateCR or crStateMixed: a nested Phi that
			// already resolved to crStateMixed still has a CR reachable,
			// and combining it with crStateUnknown or crStateNonCR must
			// not lose that fact. Unknown vs NonCR stays crStateUnknown:
			// we cannot confirm any branch is a CR, so the less alarming
			// "cannot resolve" message is more accurate.
			if result == crStateCR || s == crStateCR || result == crStateMixed || s == crStateMixed {
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

// methodParamIdxByType returns the index of the first parameter in the named
// method's signature (as declared in iface) whose type is identical to
// paramType. Returns -1 if the method is not found or has no such parameter.
func methodParamIdxByType(iface *types.Interface, methodName string, paramType types.Type) int {
	for m := range iface.Methods() {
		if m.Name() != methodName {
			continue
		}
		sig, ok := m.Type().(*types.Signature)
		if !ok {
			continue
		}
		for j := 0; j < sig.Params().Len(); j++ {
			if types.Identical(sig.Params().At(j).Type(), paramType) {
				return j
			}
		}
	}
	return -1
}

// classifyPatchSSAValue determines whether the patch argument of a Patch call
// is structurally safe (explicit fields only) or unsafe (implicit ownership).
//
// Safe:   client.RawPatch(types.JSONPatchType, …), client.RawPatch(types.ApplyPatchType, …), client.RawPatch(types.ApplyCBORPatchType, …), client.Apply
// Unsafe: client.MergeFrom, client.MergeFromWithOptions, client.StrategicMergeFrom,
//
//	client.RawPatch(types.MergePatchType, …), client.RawPatch(types.StrategicMergePatchType, …)
//
// Unknown: anything else; treated conservatively as unsafe by the caller.
//
// seen caches already-classified values and breaks Phi cycles (the same
// pattern as classifySSAValue). Pass a fresh map for each independent call site.
func classifyPatchSSAValue(v ssa.Value, seen map[ssa.Value]patchSafety, clientPkgPath string) patchSafety {
	if s, ok := seen[v]; ok {
		return s
	}
	seen[v] = patchSafetyInProgress
	s := classifyPatchSSAValueUncached(v, seen, clientPkgPath)
	seen[v] = s
	return s
}

// classifyPatchSSAValueUncached computes the safety of v. Call classifyPatchSSAValue
// instead: it adds the caching and cycle handling this function relies on.
func classifyPatchSSAValueUncached(v ssa.Value, seen map[ssa.Value]patchSafety, clientPkgPath string) patchSafety {
	switch v := v.(type) {
	case *ssa.Phi:
		// Return patchSafetySafe only when every non-back-edge is safe.
		// A back-edge into a Phi still being classified (patchSafetyInProgress)
		// contributes no verdict; the result is decided by the edges that terminate.
		allSafe, anyEdge := true, false
		for _, edge := range v.Edges {
			s := classifyPatchSSAValue(edge, seen, clientPkgPath)
			if s == patchSafetyInProgress {
				continue
			}
			anyEdge = true
			if s != patchSafetySafe {
				allSafe = false
				break
			}
		}
		if anyEdge && allSafe {
			return patchSafetySafe
		}
		return patchSafetyUnknown
	case *ssa.Call:
		callee := v.Call.StaticCallee()
		if callee == nil || callee.Package() == nil {
			return patchSafetyUnknown
		}
		if callee.Package().Pkg.Path() != clientPkgPath {
			return patchSafetyUnknown
		}
		switch callee.Name() {
		case "MergeFrom", "MergeFromWithOptions", "StrategicMergeFrom":
			return patchSafetyUnsafe
		case "RawPatch":
			if len(v.Call.Args) == 0 {
				return patchSafetyUnknown
			}
			return classifyRawPatchType(v.Call.Args[0])
		}
		return patchSafetyUnknown
	case *ssa.UnOp:
		// client.Apply is declared as: var Apply Patch = applyPatch{}
		// Loading it in SSA produces UnOp{Op: token.MUL, X: *ssa.Global{Apply}}.
		if v.Op == token.MUL {
			if g, ok := v.X.(*ssa.Global); ok &&
				g.Package() != nil &&
				g.Package().Pkg.Path() == clientPkgPath &&
				g.Name() == "Apply" {
				return patchSafetySafe
			}
		}
		return patchSafetyUnknown
	}
	return patchSafetyUnknown
}

// classifyRawPatchType inspects the first argument of client.RawPatch — the
// types.PatchType string constant — and returns the corresponding patchSafety.
// Phi nodes (patch type selected via a conditional) are classified by
// aggregating all non-back-edge verdicts: safe only when every reachable edge
// is safe. Call classifyRawPatchType instead of classifyRawPatchTypeSeen
// directly; it allocates the cycle-detection state.
//
// Safe:   JSONPatchType, ApplyYAMLPatchType, ApplyCBORPatchType
// Unsafe: MergePatchType, StrategicMergePatchType
// Unknown: anything else (treated conservatively as unsafe by the caller)
func classifyRawPatchType(v ssa.Value) patchSafety {
	return classifyRawPatchTypeSeen(v, make(map[ssa.Value]patchSafety))
}

// classifyRawPatchTypeSeen is classifyRawPatchType with explicit cycle-detection
// state. seen maps each already-classified value to its result;
// patchSafetyInProgress is the cycle-breaking sentinel for Phi back-edges.
func classifyRawPatchTypeSeen(v ssa.Value, seen map[ssa.Value]patchSafety) patchSafety {
	if s, ok := seen[v]; ok {
		return s
	}
	seen[v] = patchSafetyInProgress
	var s patchSafety
	switch v := v.(type) {
	case *ssa.Const:
		if v.Value.Kind() != goconstant.String {
			s = patchSafetyUnknown
			break
		}
		switch goconstant.StringVal(v.Value) {
		case string(ktypes.JSONPatchType), string(ktypes.ApplyYAMLPatchType), string(ktypes.ApplyCBORPatchType):
			s = patchSafetySafe
		case string(ktypes.MergePatchType), string(ktypes.StrategicMergePatchType):
			s = patchSafetyUnsafe
		default:
			s = patchSafetyUnknown
		}
	case *ssa.Phi:
		// Return patchSafetySafe only when every non-back-edge is safe.
		// A back-edge (patchSafetyInProgress) contributes no verdict; the
		// result is decided by the edges that terminate.
		allSafe, anyEdge := true, false
		for _, edge := range v.Edges {
			es := classifyRawPatchTypeSeen(edge, seen)
			if es == patchSafetyInProgress {
				continue
			}
			anyEdge = true
			if es != patchSafetySafe {
				allSafe = false
				break
			}
		}
		if anyEdge && allSafe {
			s = patchSafetySafe
		} else {
			s = patchSafetyUnknown
		}
	default:
		s = patchSafetyUnknown
	}
	seen[v] = s
	return s
}

// implementsWriter reports whether t satisfies iface.
func implementsWriter(t types.Type, iface *types.Interface) bool {
	return types.Implements(t, iface)
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
