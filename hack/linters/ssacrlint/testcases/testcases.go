// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package testcases contains the ssacrlint test fixtures. Each exported
// function exercises one scenario. The test driver in analyzer_test.go loads
// this package with analysistest and verifies that exactly the expected
// diagnostics are emitted.
//
// The package uses the real sigs.k8s.io/controller-runtime/pkg/client types so
// that the tests exercise the same interface that production code uses. The only
// "fake" element is fakeapi/pkg/apis/v1, an in-module stub whose package-path
// prefix the test overrides via the -cr-path-pattern flag to simulate ECK CR types.
package testcases

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	otherv1 "github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint/testcases/fakeapi/other/v1"
	fakev1 "github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint/testcases/fakeapi/pkg/apis/v1"
)

// FlaggedUpdate calls Update on a fake ECK CR — must produce a diagnostic.
func FlaggedUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) // want "on an ECK CR"
}

// NotFlaggedNonCR calls Update on a plain Kubernetes type — must not be flagged.
func NotFlaggedNonCR(c client.Client, ctx context.Context) {
	secret := &corev1.Secret{}
	c.Update(ctx, secret)
}

// NotFlaggedNonAPIPathType calls Update on a type that lives under the fakeapi
// module root but outside pkg/apis/. The -cr-path-pattern requires pkg/apis/
// after the module root, so this type must not be flagged.
func NotFlaggedNonAPIPathType(c client.Client, ctx context.Context) {
	obj := &otherv1.OtherType{}
	c.Update(ctx, obj)
}

// NotFlaggedStatusUpdate calls Status().Update() — SubResourceWriter does not
// implement client.Writer, so this must not be flagged.
func NotFlaggedStatusUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Status().Update(ctx, cr)
}

// FlaggedMethodExprUpdate calls Update as a method expression where the
// receiver is passed as an explicit first argument — must produce a diagnostic.
func FlaggedMethodExprUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	client.Client.Update(c, ctx, cr) // want "on an ECK CR"
}

// FlaggedAliasUpdate calls Update on a type alias of an ECK CR. The analyzer
// must unwrap the alias via types.Unalias before asserting *types.Named —
// must produce a diagnostic.
func FlaggedAliasUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCRAlias{}
	c.Update(ctx, cr) // want "on an ECK CR"
}

// FlaggedPtrAliasUpdate calls Update on a pointer-type alias of an ECK CR
// (type FakeCRPtrAlias = *FakeCR). The analyzer must handle the
// alias→pointer→named chain via the iterative unwrap loop —
// must produce a diagnostic.
func FlaggedPtrAliasUpdate(c client.Client, ctx context.Context) {
	var cr fakev1.FakeCRPtrAlias = &fakev1.FakeCR{}
	c.Update(ctx, cr) // want "on an ECK CR"
}

// FlaggedConversionUpdate passes the CR through an interface conversion
// (client.Object(cr)). TypeOf returns client.Object, not the concrete type;
// the analyzer must unwrap the conversion to detect the ECK CR —
// must produce a diagnostic.
func FlaggedConversionUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, client.Object(cr)) // want "on an ECK CR"
}

// NotFlaggedConversionNonCR wraps a plain Kubernetes type in an interface
// conversion. After unwrapping, the concrete type is not an ECK CR —
// must not be flagged.
func NotFlaggedConversionNonCR(c client.Client, ctx context.Context) {
	secret := &corev1.Secret{}
	c.Update(ctx, client.Object(secret))
}

// nonCRSource has the same underlying struct type as FakeCR but lives in
// the testcases package, which is not an ECK CR prefix. It is used as the
// source of a (*fakev1.FakeCR) conversion to verify that the analyzer
// classifies the conversion result rather than the source operand.
type nonCRSource struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

// FlaggedConcreteConversionUpdate converts a non-ECK-CR value to a concrete
// ECK CR type (*fakev1.FakeCR). The source (nonCRSource) is not itself a
// CR, so an implementation that incorrectly unwrapped the conversion would
// miss the diagnostic. The conversion result IS the CR type — must produce a diagnostic.
func FlaggedConcreteConversionUpdate(c client.Client, ctx context.Context) {
	var other nonCRSource
	c.Update(ctx, (*fakev1.FakeCR)(&other)) // want "on an ECK CR"
}

// FlaggedParenConversionUpdate wraps the interface conversion in parentheses.
// The analyzer must unwrap *ast.ParenExpr to reach the underlying conversion
// and detect the ECK CR type — must produce a diagnostic.
func FlaggedParenConversionUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, (client.Object(cr))) // want "on an ECK CR"
}

// FlaggedInterfaceVarUpdate declares obj as client.Object but assigns a
// concrete ECK CR value. The static type at the call site is client.Object,
// but the analyzer traces the variable back to its initializer and must
// produce a diagnostic.
func FlaggedInterfaceVarUpdate(c client.Client, ctx context.Context) {
	var obj client.Object = &fakev1.FakeCR{}
	c.Update(ctx, obj) // want "on an ECK CR"
}

// FlaggedReassignedInterfaceVarUpdate declares obj as client.Object with no
// initializer and then assigns a concrete ECK CR before calling Update.
// The analyzer must detect the assignment and produce a diagnostic.
func FlaggedReassignedInterfaceVarUpdate(c client.Client, ctx context.Context) {
	var obj client.Object
	obj = &fakev1.FakeCR{}
	c.Update(ctx, obj) // want "on an ECK CR"
}

// NotFlaggedInterfaceVarNonCR declares obj as client.Object but assigns a
// plain Kubernetes type. After tracing back to the initializer the concrete
// type is not an ECK CR — must not produce a diagnostic.
func NotFlaggedInterfaceVarNonCR(c client.Client, ctx context.Context) {
	var obj client.Object = &corev1.Secret{}
	c.Update(ctx, obj)
}

// FlaggedFuncParamUpdate receives obj as a client.Object parameter and passes
// it directly to Update. The parameter has no tracked concrete type in the
// package scan; the analyzer treats uncertainty as a potential violation —
// must produce a diagnostic.
func FlaggedFuncParamUpdate(c client.Client, ctx context.Context, obj client.Object) {
	c.Update(ctx, obj) // want "cannot resolve its concrete type"
}

// FlaggedPatchCR calls Patch on a fake ECK CR — must produce a diagnostic.
func FlaggedPatchCR(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.MergeFrom(&fakev1.FakeCR{})) // want "on an ECK CR"
}

// NotFlaggedPatchNonCR calls Patch on a plain Kubernetes type — must not be flagged.
func NotFlaggedPatchNonCR(c client.Client, ctx context.Context) {
	secret := &corev1.Secret{}
	c.Patch(ctx, secret, client.MergeFrom(&corev1.Secret{}))
}

// NotFlaggedSubResourcePatch calls Status().Patch() — SubResourceWriter does
// not implement client.Writer, so this must not be flagged.
func NotFlaggedSubResourcePatch(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Status().Patch(ctx, cr, client.MergeFrom(&fakev1.FakeCR{}))
}

// FlaggedPatchInterfaceVarCR declares obj as client.Object but assigns a
// concrete ECK CR, then passes obj to Patch — must produce a diagnostic
// with the concrete-CR message.
func FlaggedPatchInterfaceVarCR(c client.Client, ctx context.Context) {
	var obj client.Object = &fakev1.FakeCR{}
	c.Patch(ctx, obj, client.MergeFrom(&fakev1.FakeCR{})) // want "on an ECK CR"
}

// FlaggedPatchFuncParam receives obj as a client.Object parameter and passes
// it to Patch. The analyzer cannot resolve the concrete type and treats
// uncertainty as a potential violation — must produce the unresolved diagnostic.
func FlaggedPatchFuncParam(c client.Client, ctx context.Context, obj client.Object) {
	c.Patch(ctx, obj, client.MergeFrom(&fakev1.FakeCR{})) // want "cannot resolve its concrete type"
}

// FlaggedHomogeneousBranchUpdate assigns a fake ECK CR in both branches of a
// conditional. SSA produces a Phi node where every incoming edge is a CR —
// must produce a diagnostic with the concrete-CR message.
func FlaggedHomogeneousBranchUpdate(c client.Client, ctx context.Context, b bool) {
	var obj client.Object
	if b {
		obj = &fakev1.FakeCR{}
	} else {
		obj = &fakev1.FakeCR{}
	}
	c.Update(ctx, obj) // want "on an ECK CR"
}

// FlaggedMixedBranchUpdate assigns a CR in one branch and a non-CR in the
// other. SSA produces a Phi node with divergent edges (crStateMixed); the
// analyzer classifies the call as mixed-branch — must produce a diagnostic with
// the mixed-branch message.
func FlaggedMixedBranchUpdate(c client.Client, ctx context.Context, b bool) {
	var obj client.Object
	if b {
		obj = &fakev1.FakeCR{}
	} else {
		obj = &corev1.Secret{}
	}
	c.Update(ctx, obj) // want "on at least one branch"
}

// FlaggedNestedBranchAllCRUpdate assigns a fake ECK CR on every path through
// nested conditionals. SSA produces a Phi whose edges include both the initial
// value and an inner Phi that also has that initial value as an edge, so the
// same SSA value is reachable from two edges of the outer Phi. Classification
// must cache each value's state rather than only marking it visited, otherwise
// the second visit reports "unknown" and collapses the unanimous Phi to mixed —
// must produce a diagnostic with the concrete-CR message.
func FlaggedNestedBranchAllCRUpdate(c client.Client, ctx context.Context, a, b bool) {
	var obj client.Object = &fakev1.FakeCR{}
	if a {
		if b {
			obj = &fakev1.FakeCR{}
		}
	}
	c.Update(ctx, obj) // want "on an ECK CR"
}

// FlaggedMethodExprPatch calls Patch as a method expression where the receiver
// is passed as an explicit first argument — must produce a diagnostic.
func FlaggedMethodExprPatch(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	client.Client.Patch(c, ctx, cr, client.MergeFrom(&fakev1.FakeCR{})) // want "on an ECK CR"
}

// FlaggedMixedBranchPatch assigns a CR in one branch and a non-CR in the
// other, then passes the result to Patch. SSA produces a Phi node with
// divergent edges (crStateMixed) — must produce a diagnostic with the
// mixed-branch message.
func FlaggedMixedBranchPatch(c client.Client, ctx context.Context, b bool) {
	var obj client.Object
	if b {
		obj = &fakev1.FakeCR{}
	} else {
		obj = &corev1.Secret{}
	}
	c.Patch(ctx, obj, client.MergeFrom(&fakev1.FakeCR{})) // want "on at least one branch"
}

// FlaggedLoopCarriedPhiUpdate exercises the crStateInProgress cycle-breaking
// path in classifySSAValueUncached. The conditional assignment inside the loop
// produces a Phi pair at the loop header and the if-merge block that reference
// each other: loop_header_phi→after_if_phi→loop_header_phi. The
// crStateInProgress sentinel breaks the cycle so the classifier terminates.
// All reachable concrete types are ECK CRs — must produce a diagnostic.
func FlaggedLoopCarriedPhiUpdate(c client.Client, ctx context.Context, n int, cond bool) {
	var obj client.Object = &fakev1.FakeCR{}
	for range n {
		if cond {
			obj = &fakev1.FakeCR{}
		}
	}
	c.Update(ctx, obj) // want "on an ECK CR"
}

// FlaggedUnknownVsNonCRBranchUpdate assigns an unresolvable function parameter
// on one branch and a plain non-CR type on the other. Neither branch is a
// confirmed ECK CR, so the Phi must not produce crStateMixed ("on at least one
// branch"). The analyzer must fall back to crStateUnknown and emit the
// "cannot resolve" diagnostic instead.
func FlaggedUnknownVsNonCRBranchUpdate(c client.Client, ctx context.Context, obj client.Object, b bool) {
	var target client.Object
	if b {
		target = obj // unknown: function parameter, concrete type unresolvable
	} else {
		target = &corev1.Secret{} // non-CR
	}
	c.Update(ctx, target) // want "cannot resolve its concrete type"
}

// FlaggedMixedPhiCombinedWithUnknownUpdate nests a crStateMixed inner Phi
// (CR on one sub-branch, non-CR on the other) inside an outer Phi whose
// remaining edge is a crStateUnknown function parameter. The Phi aggregator
// must preserve crStateMixed rather than collapsing it to crStateUnknown —
// must produce the mixed-branch diagnostic.
func FlaggedMixedPhiCombinedWithUnknownUpdate(c client.Client, ctx context.Context, obj client.Object, a, b bool) {
	var target client.Object
	if a {
		if b {
			target = &fakev1.FakeCR{}
		} else {
			target = &corev1.Secret{}
		}
	} else {
		target = obj
	}
	c.Update(ctx, target) // want "on at least one branch"
}

// FlaggedMixedPhiCombinedWithNonCRUpdate nests a crStateMixed inner Phi
// inside an outer Phi whose remaining edge is a plain crStateNonCR type.
// The Phi aggregator must preserve crStateMixed rather than collapsing it
// to crStateUnknown — must produce the mixed-branch diagnostic.
func FlaggedMixedPhiCombinedWithNonCRUpdate(c client.Client, ctx context.Context, a, b bool) {
	var target client.Object
	if a {
		if b {
			target = &fakev1.FakeCR{}
		} else {
			target = &corev1.Secret{}
		}
	} else {
		target = &corev1.Secret{}
	}
	c.Update(ctx, target) // want "on at least one branch"
}

// FlaggedMethodExprInterfaceVarCR calls Update as a method expression where
// the object argument is an interface-typed variable holding a concrete ECK CR.
// The SSA map must resolve the concrete type through the MakeInterface
// instruction — must produce a diagnostic.
func FlaggedMethodExprInterfaceVarCR(c client.Client, ctx context.Context) {
	var obj client.Object = &fakev1.FakeCR{}
	client.Client.Update(c, ctx, obj) // want "on an ECK CR"
}

// NotFlaggedMethodExprInterfaceVarNonCR calls Update as a method expression
// where the object argument is an interface-typed variable holding a plain
// Kubernetes type. The SSA map must resolve the concrete type and recognise
// it is not an ECK CR — must not produce a diagnostic.
func NotFlaggedMethodExprInterfaceVarNonCR(c client.Client, ctx context.Context) {
	var obj client.Object = &corev1.Secret{}
	client.Client.Update(c, ctx, obj)
}

// NotFlaggedCreate calls Create on a fake ECK CR. Create is not in the
// monitored method list (Update and Patch only) and must not be flagged.
func NotFlaggedCreate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Create(ctx, cr)
}

// NotFlaggedDelete calls Delete on a fake ECK CR. Delete is not in the
// monitored method list (Update and Patch only) and must not be flagged.
func NotFlaggedDelete(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Delete(ctx, cr)
}

// NotFlaggedDeleteAllOf calls DeleteAllOf on a fake ECK CR. DeleteAllOf is not
// in the monitored method list (Update and Patch only) and must not be flagged.
func NotFlaggedDeleteAllOf(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.DeleteAllOf(ctx, cr)
}

// NotFlaggedPatchJSONType calls Patch with RawPatch(JSONPatchType) on an ECK CR.
// JSONPatch modifies only explicit paths — must not be flagged.
func NotFlaggedPatchJSONType(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/x","value":1}]`)))
}

// NotFlaggedPatchApplyType calls Patch with RawPatch(ApplyPatchType) on an ECK CR.
// ApplyPatch modifies only explicit fields — must not be flagged.
func NotFlaggedPatchApplyType(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.RawPatch(ktypes.ApplyPatchType, []byte(`{"spec":{"x":1}}`)))
}

// NotFlaggedPatchApplyCBORType calls Patch with RawPatch(ApplyCBORPatchType) on an ECK CR.
// CBOR-encoded SSA apply modifies only explicit fields — must not be flagged.
func NotFlaggedPatchApplyCBORType(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.RawPatch(ktypes.ApplyCBORPatchType, []byte(`\xa1dspec\xa1ax\x01`)))
}

// NotFlaggedPatchApply calls Patch with client.Apply on an ECK CR.
// client.Apply modifies only explicit fields — must not be flagged.
func NotFlaggedPatchApply(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.Apply)
}

// FlaggedPatchMergePatchType calls Patch with RawPatch(MergePatchType) on an ECK CR.
// MergePatch claims all fields present in the diff — must be flagged.
func FlaggedPatchMergePatchType(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.RawPatch(ktypes.MergePatchType, []byte(`{"spec":{}}`))) // want "on an ECK CR"
}

// FlaggedPatchMergePatchTypeInterfaceVar calls Patch with RawPatch(MergePatchType) where the
// object is a client.Object variable holding an ECK CR — must be flagged.
func FlaggedPatchMergePatchTypeInterfaceVar(c client.Client, ctx context.Context) {
	var obj client.Object = &fakev1.FakeCR{}
	c.Patch(ctx, obj, client.RawPatch(ktypes.MergePatchType, []byte(`{"spec":{}}`))) // want "on an ECK CR"
}

// FlaggedPatchMergePatchTypeFuncParam calls Patch with RawPatch(MergePatchType) where the
// object is an unresolvable function parameter — must produce the "cannot resolve" diagnostic.
func FlaggedPatchMergePatchTypeFuncParam(c client.Client, ctx context.Context, obj client.Object) {
	c.Patch(ctx, obj, client.RawPatch(ktypes.MergePatchType, []byte(`{"spec":{}}`))) // want "cannot resolve its concrete type"
}

// NotFlaggedPatchJSONTypeInterfaceVar calls Patch with RawPatch(JSONPatchType) where the
// object is a client.Object variable holding an ECK CR. The patch type is safe regardless
// of what is in the object argument — must not be flagged.
func NotFlaggedPatchJSONTypeInterfaceVar(c client.Client, ctx context.Context) {
	var obj client.Object = &fakev1.FakeCR{}
	c.Patch(ctx, obj, client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/x","value":1}]`)))
}

// NotFlaggedPatchJSONTypeFuncParam calls Patch with RawPatch(JSONPatchType) where the
// object is an unresolvable function parameter. The patch type is safe regardless of
// whether the object is an ECK CR — must not be flagged.
func NotFlaggedPatchJSONTypeFuncParam(c client.Client, ctx context.Context, obj client.Object) {
	c.Patch(ctx, obj, client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/x","value":1}]`)))
}

// NotFlaggedPatchJSONTypeLocalVar stores a JSONPatch in a local variable before
// passing it to Patch. SSA keeps the variable in SSA form (no alloc), so the patch
// argument is still the *ssa.Call result — must not be flagged.
func NotFlaggedPatchJSONTypeLocalVar(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	p := client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/spec/x","value":1}]`))
	c.Patch(ctx, cr, p)
}

// NotFlaggedPatchApplyLocalVar stores client.Apply in a local variable before
// passing it to Patch. SSA represents Apply as *ssa.UnOp{MUL, Global{Apply}},
// which classifyPatchSSAValue already handles — must not be flagged.
func NotFlaggedPatchApplyLocalVar(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	p := client.Apply
	c.Patch(ctx, cr, p)
}

// NotFlaggedPatchJSONTypePhiBothSafe stores a JSONPatch via a conditional (producing
// a Phi node) where both branches use safe patch types. classifyPatchSSAValue must
// recognise a Phi whose every edge is patchSafetySafe as safe — must not be flagged.
func NotFlaggedPatchJSONTypePhiBothSafe(c client.Client, ctx context.Context, b bool) {
	cr := &fakev1.FakeCR{}
	var p client.Patch
	if b {
		p = client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/a","value":1}]`))
	} else {
		p = client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/b","value":2}]`))
	}
	c.Patch(ctx, cr, p)
}

// FlaggedPatchPhiMixedSafety stores a patch via a conditional where one branch is
// safe (JSONPatch) and the other is unsafe (MergeFrom). The Phi cannot be confirmed
// safe, so classifyPatchSSAValue returns unknown and the CR check runs —
// must produce a diagnostic.
func FlaggedPatchPhiMixedSafety(c client.Client, ctx context.Context, b bool) {
	cr := &fakev1.FakeCR{}
	var p client.Patch
	if b {
		p = client.RawPatch(ktypes.JSONPatchType, []byte(`[{"op":"replace","path":"/a","value":1}]`))
	} else {
		p = client.MergeFrom(&fakev1.FakeCR{})
	}
	c.Patch(ctx, cr, p) // want "on an ECK CR"
}

// FlaggedPatchStrategicMergeFrom calls Patch with client.StrategicMergeFrom on an ECK CR.
// StrategicMergeFrom uses implicit field ownership — must be flagged.
func FlaggedPatchStrategicMergeFrom(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.StrategicMergeFrom(&fakev1.FakeCR{})) // want "on an ECK CR"
}

// FlaggedPatchMergeFromWithOptions calls Patch with client.MergeFromWithOptions on an ECK CR.
// MergeFromWithOptions uses implicit field ownership — must be flagged.
func FlaggedPatchMergeFromWithOptions(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.MergeFromWithOptions(&fakev1.FakeCR{})) // want "on an ECK CR"
}

// FlaggedPatchStrategicMergePatchType calls Patch with RawPatch(StrategicMergePatchType) on an ECK CR.
// Strategic merge patch claims all fields present in the diff — must be flagged.
func FlaggedPatchStrategicMergePatchType(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.RawPatch(ktypes.StrategicMergePatchType, []byte(`{"spec":{}}`))) // want "on an ECK CR"
}

// NotFlaggedPatchRawPatchTypePhiBothSafe stores two different known-safe patch
// type constants in a conditional, producing a Phi node over two *ssa.Const
// values for the RawPatch type argument. classifyRawPatchTypeSeen must recurse
// into the Phi and recognise that every edge is safe — must not produce a
// diagnostic.
func NotFlaggedPatchRawPatchTypePhiBothSafe(c client.Client, ctx context.Context, b bool) {
	cr := &fakev1.FakeCR{}
	var pt ktypes.PatchType
	if b {
		pt = ktypes.JSONPatchType
	} else {
		pt = ktypes.ApplyPatchType
	}
	c.Patch(ctx, cr, client.RawPatch(pt, []byte(`[{"op":"replace","path":"/spec/x","value":1}]`)))
}

// FlaggedPatchRawPatchTypePhiMixed stores a conditional where one branch is a
// safe patch type (JSONPatchType) and the other is unsafe (MergePatchType),
// producing a Phi node for the RawPatch type argument. The Phi cannot be
// confirmed safe — must produce a diagnostic.
func FlaggedPatchRawPatchTypePhiMixed(c client.Client, ctx context.Context, b bool) {
	cr := &fakev1.FakeCR{}
	var pt ktypes.PatchType
	if b {
		pt = ktypes.JSONPatchType
	} else {
		pt = ktypes.MergePatchType
	}
	c.Patch(ctx, cr, client.RawPatch(pt, []byte(`{"spec":{}}`))) // want "on an ECK CR"
}

// NotFlaggedMethodExprPatchInterfaceVarNonCR calls Patch as a method expression
// where the object argument is an interface-typed variable holding a plain
// Kubernetes type. The SSA map must resolve the concrete type and recognise
// it is not an ECK CR — must not produce a diagnostic.
func NotFlaggedMethodExprPatchInterfaceVarNonCR(c client.Client, ctx context.Context) {
	var obj client.Object = &corev1.Secret{}
	client.Client.Patch(c, ctx, obj, client.MergeFrom(&corev1.Secret{}))
}

// writerImpl is a minimal concrete client.Writer implementation. It exists so
// that tests can use pointer-to-concrete-type method expressions
// ((*writerImpl).Update) to exercise the non-thunk SSA path through
// computeSSACallData, complementing the interface method-expression thunk path
// already covered by FlaggedMethodExprUpdate.
type writerImpl struct{}

func (writerImpl) Apply(_ context.Context, _ runtime.ApplyConfiguration, _ ...client.ApplyOption) error {
	return nil
}
func (writerImpl) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return nil
}
func (writerImpl) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return nil
}
func (writerImpl) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return nil
}
func (writerImpl) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return nil
}
func (writerImpl) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}

var _ client.Writer = (*writerImpl)(nil)

// FlaggedConcreteTypeMethodExprUpdate calls Update via a concrete-type method
// expression (writerImpl.Update). Unlike the interface-type method expression
// tested by FlaggedMethodExprUpdate (client.Client.Update, which produces a
// "$thunk" SSA callee), this form produces a direct SSA static call whose
// callee name is "(writerImpl).Update". normalizeSSAMethodName must strip the
// type prefix — must produce a diagnostic.
func FlaggedConcreteTypeMethodExprUpdate(ctx context.Context) {
	cr := &fakev1.FakeCR{}
	w := writerImpl{}
	writerImpl.Update(w, ctx, cr) // want "on an ECK CR"
}
