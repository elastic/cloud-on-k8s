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
	// LatestReleasedVersion6x is the latest released version for 6.x
	LatestReleasedVersion6x = "6.8.23"
	// LatestReleasedVersion7x is the latest released version for 7.x
	LatestReleasedVersion7x = "7.17.1"
	// LatestReleasedVersion8x is the latest release version for 8.x
	LatestReleasedVersion8x = "8.1.1"
	// LatestSnapshotVersion8x is the latest snapshot version for 8.x
	LatestSnapshotVersion8x = "8.2.0-SNAPSHOT"
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
