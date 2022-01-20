// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

// Elastic Stack versions used in the E2E tests. These should be updated as new versions for each major are released.
const (
	// LatestVersion6x is the current latest production version for 6.x
	LatestVersion6x = "6.8.20"
	// LatestVersion7x is the current latest production version for 7.x
	LatestVersion7x = "7.16.2"
	// LatestVersion8x is the current latest snapshot version for 8.x
	LatestVersion8x = "8.0.0-SNAPSHOT"
)

// SkipInvalidUpgrade skips a test that would do an invalid upgrade.
func SkipInvalidUpgrade(t *testing.T, srcVersion string, dstVersion string) {
	t.Helper()
	isValid, err := isValidUpgrade(srcVersion, dstVersion)
	if err != nil {
		t.Fatalf("Failed to determine the validity of the upgrade path: %v", err)
	}
	if !isValid {
		t.SkipNow()
	}
}

// isValidUpgrade reports whether an upgrade from one version to another version is valid.
func isValidUpgrade(from string, to string) (bool, error) {
	srcVer, err := version.Parse(from)
	if err != nil {
		return false, fmt.Errorf("failed to parse version '%s': %w", from, err)
	}
	dstVer, err := version.Parse(to)
	if srcVer.Pre != nil && dstVer.Pre == nil {
		// an upgrade from a pre-release version to a released version must not be tested (mainly due to incompatible licensing)
		// but an upgrade from a released version to a pre-release version is to be tested (to catch any new issues before the release)
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to parse version '%s': %w", to, err)
	}

	// downgrades are not supported
	if srcVer.GTE(dstVer) {
		return false, nil
	}

	// upgrades within the same major are always ok
	if srcVer.Major == dstVer.Major {
		return true, nil
	}

	// special case of major upgrade: last minor of major 6 to any major 7
	latest6x := version.MustParse(LatestVersion6x)
	if srcVer.Major == 6 && srcVer.Minor == latest6x.Minor && dstVer.Major == 7 {
		return true, nil
	}

	// special case of major upgrade: last minor of major 7 to any major 8
	latest7x := version.MustParse(LatestVersion7x)
	if srcVer.Major == 7 && srcVer.Minor == latest7x.Minor && dstVer.Major == 8 {
		return true, nil
	}

	// all valid cases are capture above
	return false, nil
}
