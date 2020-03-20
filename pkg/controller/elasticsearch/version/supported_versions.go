// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	pkgerrors "github.com/pkg/errors"
)

// LowestHighestSupportedVersions expresses the wire-format compatibility range for a version.
type LowestHighestSupportedVersions struct {
	LowestSupportedVersion  version.Version
	HighestSupportedVersion version.Version
}

// SupportedVersions returns the supported minor versions for given major version
func SupportedVersions(v version.Version) *LowestHighestSupportedVersions {
	switch v.Major {
	case 6:
		return &LowestHighestSupportedVersions{
			// Min. version is 6.8.0.
			LowestSupportedVersion: version.MustParse("6.8.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("6.99.99"),
		}
	case 7:
		return &LowestHighestSupportedVersions{
			// 6.8.0 is the lowest wire compatibility version for 7.x
			LowestSupportedVersion: version.MustParse("6.8.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			HighestSupportedVersion: version.MustParse("7.99.99"),
		}
	case 8:
		return &LowestHighestSupportedVersions{
			// 7.4.0 is the lowest version that offers a direct upgrade path to 8.0
			LowestSupportedVersion:  version.MustParse("7.4.0"),
			HighestSupportedVersion: version.MustParse("8.99.99"),
		}
	default:
		return nil
	}
}

// Supports compares a given with the supported version range and returns an error if out of bounds.
func (lh LowestHighestSupportedVersions) Supports(v version.Version) error {
	if !v.IsSameOrAfter(lh.LowestSupportedVersion) {
		return pkgerrors.Errorf(
			"%s is unsupported, it is older than the oldest supported version %s",
			v,
			lh.LowestSupportedVersion,
		)
	}

	if !lh.HighestSupportedVersion.IsSameOrAfter(v) {
		return pkgerrors.Errorf(
			"%s is unsupported, it is newer than the newest supported version %s",
			v,
			lh.HighestSupportedVersion,
		)
	}
	return nil
}
