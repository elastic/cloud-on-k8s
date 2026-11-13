// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

// Elastic Stack versions used in the E2E tests. These should be updated as new versions for each major are released.
const (
	// LatestReleasedVersion7x is the latest released version for 7.x
	LatestReleasedVersion7x = "7.17.29"
	// LatestReleasedVersion8x is the latest release version for 8.x
	LatestReleasedVersion8x = "8.19.20"
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

	SkipUnsupportedStackVariantVersions(t, srcVersion, dstVersion)
}

func SkipUnsupportedStackVariantVersions(t *testing.T, versions ...string) {
	t.Helper()

	if Ctx().ContainerSuffix != container.WolfiSuffix {
		return
	}

	for _, v := range versions {
		ver, err := version.Parse(v)
		if err != nil {
			t.Fatalf("SkipUnsupportedStackVariantVersions: failed to parse version %q: %v", v, err)
		}
		if supported, reason := isVersionSupportedForWolfi(ver); !supported {
			t.Skipf("%s", reason)
		}
	}
}

// isVersionSupportedForWolfi reports whether the given version has Wolfi image variants that ECK can use.
// All stack components publish a -wolfi image variant starting with 8.16.0. However, Logstash lacks the
// openssl binary needed by ECK to inject TLS certificates until 8.16.4, 8.17.2, and 8.18+; without it
// the operator cannot bring up a Logstash instance in a TLS-enabled cluster (see https://github.com/elastic/logstash/issues/16965).
// APM does not offer a shell in any Wolfi variant version, but this affects only secure settings functionality;
// the respective tests are skipped individually.
func isVersionSupportedForWolfi(ver version.Version) (bool, string) {
	switch {
	case ver.Major == 7:
		return false, fmt.Sprintf("version %s is unsupported for Wolfi image variants", ver)
	case ver.Major == 8 && ver.Minor < 16:
		return false, fmt.Sprintf("version %s is unsupported for Wolfi image variants", ver)
	case ver.Major == 8 && ver.Minor == 16 && ver.Patch < 4:
		return false, fmt.Sprintf("version %s is unsupported for Wolfi image variants", ver)
	case ver.Major == 8 && ver.Minor == 17 && ver.Patch < 2:
		return false, fmt.Sprintf("version %s is unsupported for Wolfi image variants", ver)
	}
	return true, ""
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
