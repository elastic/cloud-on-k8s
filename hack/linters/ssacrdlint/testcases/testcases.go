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
// test overrides via the -crd-pkg-prefix flag to simulate ECK CRD types.
package testcases

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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
