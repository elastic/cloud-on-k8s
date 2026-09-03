// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package testcases contains the ssacrdlint test fixtures. Each exported
// function exercises one scenario. The test driver in analyzer_test.go loads
// this package with go/packages (module mode) and verifies that exactly the
// expected diagnostics are emitted.
//
// The package uses the real sigs.k8s.io/controller-runtime/pkg/client types so
// that the tests exercise the same interface that production code uses. The only
// "fake" element is fakeapi/v1, an in-module stub whose package-path prefix the
// test overrides via the -crd-module-prefix flag to simulate ECK CRD types.
package testcases

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fakev1 "github.com/elastic/cloud-on-k8s/hack/linters/ssacrdlint/testcases/fakeapi/v1"
)

// FlaggedUpdate calls Update on a fake ECK CRD — must produce a diagnostic.
func FlaggedUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd)
}

// NotFlaggedNonCRD calls Update on a plain Kubernetes type — must not be flagged.
func NotFlaggedNonCRD(c client.Client, ctx context.Context) {
	secret := &corev1.Secret{}
	c.Update(ctx, secret)
}

// NotFlaggedStatusUpdate calls Status().Update() — SubResourceWriter does not
// implement client.Writer, so this must not be flagged.
func NotFlaggedStatusUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Status().Update(ctx, crd)
}

// NotFlaggedNolint has a //nolint:ssacrdlint comment — must not be flagged.
func NotFlaggedNolint(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd) //nolint:ssacrdlint
}

// NotFlaggedNolintAll has a //nolint:all comment — must not be flagged.
func NotFlaggedNolintAll(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd) //nolint:all
}

// NotFlaggedBareNolint has a bare //nolint comment — must not be flagged.
func NotFlaggedBareNolint(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd) //nolint
}

// NotFlaggedMultilineNolint has //nolint:ssacrdlint on the opening line of a
// multi-line call — must not be flagged.
func NotFlaggedMultilineNolint(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update( //nolint:ssacrdlint
		ctx,
		crd,
	)
}

// NotFlaggedCombinedNolint has a //nolint:govet,ssacrdlint comment — must not be flagged.
func NotFlaggedCombinedNolint(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd) //nolint:govet,ssacrdlint
}

// FlaggedMultilineNolintWrongLine has //nolint:ssacrdlint on an argument line
// of a multi-line call. Diagnostics are reported at the call's opening line, so
// the nolint is invisible there — must produce a diagnostic.
func FlaggedMultilineNolintWrongLine(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(
		ctx,
		crd, //nolint:ssacrdlint
	)
}

// NotFlaggedPrecedingLineNolint has //nolint:ssacrdlint on the line immediately
// before the call — the repository's established suppression form — must not be flagged.
func NotFlaggedPrecedingLineNolint(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	//nolint:ssacrdlint
	c.Update(ctx, crd)
}

// FlaggedTrailingNolintPreviousStatement has //nolint:ssacrdlint trailing on
// the statement immediately before the Update call. The directive belongs to
// that statement, not the call — must produce a diagnostic.
func FlaggedTrailingNolintPreviousStatement(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	_ = crd //nolint:ssacrdlint
	c.Update(ctx, crd)
}

// FlaggedMethodExprUpdate calls Update as a method expression where the
// receiver is passed as an explicit first argument — must produce a diagnostic.
func FlaggedMethodExprUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	client.Client.Update(c, ctx, crd)
}

// FlaggedNolintUnrelated has //nolintlint on the call line, which is an
// unrelated directive and must not suppress this linter — must produce a diagnostic.
func FlaggedNolintUnrelated(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, crd) //nolintlint
}

// FlaggedAliasUpdate calls Update on a type alias of an ECK CRD. The analyzer
// must unwrap the alias via types.Unalias before asserting *types.Named —
// must produce a diagnostic.
func FlaggedAliasUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRDAlias{}
	c.Update(ctx, crd)
}

// FlaggedPtrAliasUpdate calls Update on a pointer-type alias of an ECK CRD
// (type FakeCRDPtrAlias = *FakeCRD). The analyzer must handle the
// alias→pointer→named chain via the iterative unwrap loop —
// must produce a diagnostic.
func FlaggedPtrAliasUpdate(c client.Client, ctx context.Context) {
	var crd fakev1.FakeCRDPtrAlias = &fakev1.FakeCRD{}
	c.Update(ctx, crd)
}

// FlaggedConversionUpdate passes the CRD through an interface conversion
// (client.Object(crd)). TypeOf returns client.Object, not the concrete type;
// the analyzer must unwrap the conversion to detect the ECK CRD —
// must produce a diagnostic.
func FlaggedConversionUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, client.Object(crd))
}

// NotFlaggedConversionNonCRD wraps a plain Kubernetes type in an interface
// conversion. After unwrapping, the concrete type is not an ECK CRD —
// must not be flagged.
func NotFlaggedConversionNonCRD(c client.Client, ctx context.Context) {
	secret := &corev1.Secret{}
	c.Update(ctx, client.Object(secret))
}

// nonCRDSource has the same underlying struct type as FakeCRD but lives in
// the testcases package, which is not an ECK CRD prefix. It is used as the
// source of a (*fakev1.FakeCRD) conversion to verify that the analyzer
// classifies the conversion result rather than the source operand.
type nonCRDSource struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

// FlaggedConcreteConversionUpdate converts a non-ECK-CRD value to a concrete
// ECK CRD type (*fakev1.FakeCRD). The source (nonCRDSource) is not itself a
// CRD, so an implementation that incorrectly unwrapped the conversion would
// miss the diagnostic. The conversion result IS the CRD type — must produce a diagnostic.
func FlaggedConcreteConversionUpdate(c client.Client, ctx context.Context) {
	var other nonCRDSource
	c.Update(ctx, (*fakev1.FakeCRD)(&other))
}

// FlaggedParenConversionUpdate wraps the interface conversion in parentheses.
// The analyzer must unwrap *ast.ParenExpr to reach the underlying conversion
// and detect the ECK CRD type — must produce a diagnostic.
func FlaggedParenConversionUpdate(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	c.Update(ctx, (client.Object(crd)))
}

// FlaggedBraceNolintPreviousLine has //nolint:ssacrdlint trailing after an
// opening brace on the line immediately preceding the Update call. Since code
// precedes the directive on that line it is not a standalone suppression —
// must produce a diagnostic.
func FlaggedBraceNolintPreviousLine(c client.Client, ctx context.Context) {
	crd := &fakev1.FakeCRD{}
	if true { //nolint:ssacrdlint
		c.Update(ctx, crd)
	}
}
