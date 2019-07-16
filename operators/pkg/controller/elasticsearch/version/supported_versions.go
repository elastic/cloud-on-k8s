// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	corev1 "k8s.io/api/core/v1"
)

// LowestHighestSupportedVersions expresses the wire-format compatibility range for a version.
type LowestHighestSupportedVersions struct {
	LowestSupportedVersion  version.Version
	HighestSupportedVersion version.Version
}

// VerifySupportsExistingPods checks the given pods against the supported version range in lh.
func (lh LowestHighestSupportedVersions) VerifySupportsExistingPods(
	pods []corev1.Pod,
) error {
	for _, pod := range pods {
		v, err := label.ExtractVersion(pod)
		if err != nil {
			return err
		}
		if err := lh.Supports(*v); err != nil {
			return errors.Wrapf(err, "%s has incompatible version", pod.Name)
		}
	}
	return nil
}

// Supports compares a given with the supported version range and returns an error if out of bounds.
func (lh LowestHighestSupportedVersions) Supports(v version.Version) error {
	if !v.IsSameOrAfter(lh.LowestSupportedVersion) {
		return fmt.Errorf(
			"%s is unsupported, it is older than the oldest supported version %s",
			v,
			lh.LowestSupportedVersion,
		)
	}

	if !lh.HighestSupportedVersion.IsSameOrAfter(v) {
		return fmt.Errorf(
			"%s is unsupported, it is newer than the newest supported version %s",
			v,
			lh.HighestSupportedVersion,
		)
	}
	return nil
}
