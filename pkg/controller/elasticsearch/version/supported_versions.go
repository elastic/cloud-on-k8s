// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"

// SupportedVersions returns the supported minor versions for given major version
func SupportedVersions(v version.Version) *version.MinMaxVersion {
	return supportedVersionsWithMinimum(v, version.GlobalMinStackVersion)
}

func supportedVersionsWithMinimum(v version.Version, minVersion version.Version) *version.MinMaxVersion {
	if minVersion.GT(v) {
		return nil
	}
	return technicallySupportedVersions(v)
}

func technicallySupportedVersions(v version.Version) *version.MinMaxVersion {
	switch v.Major {
	case 7:
		return &version.MinMaxVersion{
			// 7.0.0 is the minimum supported version for 7.x
			Min: version.MustParse("7.0.0"),
			Max: version.MustParse("7.99.99"),
		}
	case 8:
		return &version.MinMaxVersion{
			// 7.17.0 is the lowest version that offers a direct upgrade path to 8.0
			Min: version.MinFor(7, 17, 0), // allow snapshot builds here for testing
			Max: version.MustParse("8.99.99"),
		}
	case 9:
		return &version.MinMaxVersion{
			// 8.18.0 is the lowest version that offers a direct upgrade path to 9.0
			Min: version.MinFor(8, 18, 0), // allow snapshot builds here for testing
			Max: version.MustParse("9.99.99"),
		}
	default:
		return nil
	}
}
