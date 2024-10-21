// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

// Elastic Stack versions used in the E2E tests. These should be updated as new versions for each major are released.
const (
	// LatestReleasedVersion6x is the latest released version for 6.x
	LatestReleasedVersion6x = "6.8.23"
	// LatestReleasedVersion7x is the latest released version for 7.x
	LatestReleasedVersion7x = "7.17.24"
	// LatestReleasedVersion8x is the latest release version for 8.x
	LatestReleasedVersion8x = "8.15.3"
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
	if srcVer.Major == 6 && srcVer.Minor == 8 && dstVer.Major == 7 {
		return true, nil
	}

	// special case of major upgrade: last minor of major 7 to any major 8
	if srcVer.Major == 7 && srcVer.Minor == 17 && dstVer.Major == 8 {
		return true, nil
	}

	// all valid cases are capture above
	return false, nil
}

// GetUpgradePathTo8x returns the source and destination versions to test an upgrade to 8x. The default upgrade path
// is from the current Elastic Stack version to the latest released version 8x. However, if the current version is greater
// than the latest released version 8x (happens when the current version is the latest snapshot version 8x), then the
// upgrade path is reversed.
func GetUpgradePathTo8x(currentVersion string) (string, string) {
	if version.MustParse(currentVersion).GT(version.MustParse(LatestReleasedVersion8x)) {
		return LatestReleasedVersion8x, currentVersion
	}
	return currentVersion, LatestReleasedVersion8x
}
