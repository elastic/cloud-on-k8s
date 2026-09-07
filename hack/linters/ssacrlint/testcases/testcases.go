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

// NotFlaggedNolint has a //nolint:ssacrlint comment — must not be flagged.
func NotFlaggedNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolint:ssacrlint
}

// NotFlaggedNolintAll has a //nolint:all comment — must not be flagged.
func NotFlaggedNolintAll(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolint:all
}

// NotFlaggedBareNolint has a bare //nolint comment — must not be flagged.
func NotFlaggedBareNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolint
}

// NotFlaggedMultilineNolint has //nolint:ssacrlint on the opening line of a
// multi-line call — must not be flagged.
func NotFlaggedMultilineNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update( //nolint:ssacrlint
		ctx,
		cr,
	)
}

// NotFlaggedCombinedNolint has a //nolint:govet,ssacrlint comment — must not be flagged.
func NotFlaggedCombinedNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolint:govet,ssacrlint
}

// FlaggedMultilineNolintWrongLine has //nolint:ssacrlint on an argument line
// of a multi-line call. Diagnostics are reported at the call's opening line, so
// the nolint is invisible there — must produce a diagnostic.
func FlaggedMultilineNolintWrongLine(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update( // want "on an ECK CR"
		ctx,
		cr, //nolint:ssacrlint
	)
}

// NotFlaggedPrecedingLineNolint has //nolint:ssacrlint on the line immediately
// before the call — the repository's established suppression form — must not be flagged.
func NotFlaggedPrecedingLineNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	//nolint:ssacrlint
	c.Update(ctx, cr)
}

// FlaggedTrailingNolintPreviousStatement has //nolint:ssacrlint trailing on
// the statement immediately before the Update call. The directive belongs to
// that statement, not the call — must produce a diagnostic.
func FlaggedTrailingNolintPreviousStatement(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	_ = cr            //nolint:ssacrlint
	c.Update(ctx, cr) // want "on an ECK CR"
}

// FlaggedMethodExprUpdate calls Update as a method expression where the
// receiver is passed as an explicit first argument — must produce a diagnostic.
func FlaggedMethodExprUpdate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	client.Client.Update(c, ctx, cr) // want "on an ECK CR"
}

// FlaggedNolintUnrelated has //nolintlint on the call line. It is an
// unrelated directive (isNolintSuppressed returns false for it) and must
// not suppress this linter — must produce a diagnostic.
func FlaggedNolintUnrelated(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolintlint // want "on an ECK CR"
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

// FlaggedBraceNolintPreviousLine has //nolint:ssacrlint trailing after an
// opening brace on the line immediately preceding the Update call. Since code
// precedes the directive on that line it is not a standalone suppression —
// must produce a diagnostic.
func FlaggedBraceNolintPreviousLine(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	if true { //nolint:ssacrlint
		c.Update(ctx, cr) // want "on an ECK CR"
	}
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

// NotFlaggedPatchNolint has a //nolint:ssacrlint comment — must not be flagged.
func NotFlaggedPatchNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Patch(ctx, cr, client.MergeFrom(&fakev1.FakeCR{})) //nolint:ssacrlint
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

// NotFlaggedSpaceAfterColonNolint has //nolint: ssacrlint (space after the colon),
// which golangci-lint accepts natively. The custom isNolintSuppressed must also
// accept it — must not be flagged.
func NotFlaggedSpaceAfterColonNolint(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Update(ctx, cr) //nolint: ssacrlint
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

// NotFlaggedCreate calls Create on a fake ECK CR. Create is not in the
// monitored method list (Update and Patch only) and must not be flagged.
func NotFlaggedCreate(c client.Client, ctx context.Context) {
	cr := &fakev1.FakeCR{}
	c.Create(ctx, cr)
}
