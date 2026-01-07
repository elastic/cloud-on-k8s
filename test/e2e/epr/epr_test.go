// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build epr || e2e

package epr

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/epr"
)

func TestPackageRegistryStandalone(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	// The `lite-` variant of the package registry image was introduced in 8.15.1.
	if v.LT(version.MinFor(8, 15, 1)) {
		t.SkipNow()
	}

	name := "test-epr-standalone"
	eprBuilder := epr.NewBuilder(name).
		WithNodeCount(1).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, eprBuilder).RunSequential(t)
}

func TestPackageRegistryTLSDisabled(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	// The `lite-` variant of the package registry image was introduced in 8.15.1.
	if v.LT(version.MinFor(8, 15, 1)) {
		t.SkipNow()
	}

	name := "test-epr-tls-disabled"
	eprBuilder := epr.NewBuilder(name).
		WithNodeCount(1).
		WithTLSDisabled(true).
		WithRestrictedSecurityContext()

	test.Sequence(nil, test.EmptySteps, eprBuilder).RunSequential(t)
}

func TestPackageRegistryVersionUpgradeToLatest8x(t *testing.T) {
	v := version.MustParse(test.Ctx().ElasticStackVersion)

	// The `lite-` variant of the package registry image was introduced in 8.15.1.
	if v.LT(version.MinFor(8, 15, 1)) {
		t.SkipNow()
	}

	srcVersion, dstVersion := test.GetUpgradePathTo8x(test.Ctx().ElasticStackVersion)

	test.SkipInvalidUpgrade(t, srcVersion, dstVersion)

	name := "test-epr-version-upgrade-8x"
	epr := epr.NewBuilder(name).
		WithNodeCount(2).
		WithVersion(srcVersion).
		WithRestrictedSecurityContext()

	eprUpgraded := epr.WithVersion(dstVersion).WithMutatedFrom(&epr)

	test.RunMutations(t, []test.Builder{epr}, []test.Builder{eprUpgraded})
}
