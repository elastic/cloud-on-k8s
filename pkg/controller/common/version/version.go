// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package version

import (
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Version is an alias for semver.Version.
// It exists purely to avoid accidentally importing the old github.com/blang/semver instead of github.com/blang/semver/v4.
type Version = semver.Version

// GlobalMinStackVersion is an additional restriction on top of the technical requirements expressed below.
// Used to support UBI images where compatible images are only available from a particular stack version onwards.
var GlobalMinStackVersion Version

// supported Stack versions. See https://www.elastic.co/support/matrix#matrix_compatibility
var (
	SupportedAPMServerVersions        = MinMaxVersion{Min: From(6, 2, 0), Max: From(8, 99, 99)}
	SupportedEnterpriseSearchVersions = MinMaxVersion{Min: From(7, 7, 0), Max: From(8, 99, 99)}
	SupportedKibanaVersions           = MinMaxVersion{Min: From(6, 8, 0), Max: From(8, 99, 99)}
	SupportedBeatVersions             = MinMaxVersion{Min: From(7, 0, 0), Max: From(8, 99, 99)}
	// Elastic Agent was introduced in 7.8.0, but as "experimental release" with no migration path forward, hence
	// picking higher version as minimal supported.
	SupportedAgentVersions = MinMaxVersion{Min: From(7, 10, 0), Max: From(8, 99, 99)}
	// Due to bugfixes present in 7.14 that ECK depends on, this is the lowest version we support in Fleet mode.
	SupportedFleetModeAgentVersions = MinMaxVersion{Min: MustParse("7.14.0-SNAPSHOT"), Max: From(8, 99, 99)}
	SupportedMapsVersions           = MinMaxVersion{Min: From(7, 11, 0), Max: From(8, 99, 99)}

	// minPreReleaseVersion is the lowest prerelease identifier as numeric prerelease takes precedence before
	// alphanumeric ones and it can't have leading zeros.
	minPreReleaseVersion = mustNewPRVersion("1")
)

// MinMaxVersion holds the minimum and maximum supported versions.
// Could be replaced with semver.Range if we didn't have to support GlobalMinStackVersion.
type MinMaxVersion struct {
	Min Version
	Max Version
}

// WithinRange returns an error if the given version is not within the range of minimum and maximum versions.
func (mmv MinMaxVersion) WithinRange(v Version) error {
	if v.LT(mmv.Min) {
		return fmt.Errorf("version %s is lower than the lowest supported version of %s", v, mmv.Min)
	}

	if v.GT(mmv.Max) {
		return fmt.Errorf("version %s is higher than the highest supported version of %s", v, mmv.Max)
	}

	return nil
}

func (mmv MinMaxVersion) WithMin(min Version) MinMaxVersion {
	if min.GT(mmv.Min) {
		return MinMaxVersion{
			Min: min,
			Max: mmv.Max,
		}
	}
	return mmv
}

// Parse attempts to parse a version string.
func Parse(v string) (Version, error) {
	return semver.Parse(v)
}

// MustParse attempts to parse a version string and panics if it fails.
func MustParse(v string) Version {
	return semver.MustParse(v)
}

// From creates a new version from the given major, minor, patch numbers.
func From(major, minor, patch int) Version {
	return Version{Major: uint64(major), Minor: uint64(minor), Patch: uint64(patch)}
}

// MinFor creates a new version for the given major, minor, patch numbers with lowest PreRelease version, ie.
// the returned Version is the lowest possible version with those major, minor and patch numbers.
// See https://semver.org/#spec-item-11.
func MinFor(major, minor, patch uint64) Version {
	return Version{Major: major, Minor: minor, Patch: patch, Pre: []semver.PRVersion{minPreReleaseVersion}}
}

// MinInPods returns the lowest version parsed from labels in the given Pods.
func MinInPods(pods []corev1.Pod, labelName string) (*Version, error) {
	var min *Version
	for _, p := range pods {
		v, err := FromLabels(p.Labels, labelName)
		if err != nil {
			return nil, err
		}

		if min == nil || v.LT(*min) {
			min = &v
		}
	}

	return min, nil
}

// MinInStatefulSets returns the lowest version parsed from labels in the given StatefulSets template.
func MinInStatefulSets(ssets []appsv1.StatefulSet, labelName string) (*Version, error) {
	var min *Version
	for _, s := range ssets {
		v, err := FromLabels(s.Spec.Template.Labels, labelName)
		if err != nil {
			return nil, err
		}

		if min == nil || v.LT(*min) {
			min = &v
		}
	}

	return min, nil
}

func FromLabels(labels map[string]string, labelName string) (Version, error) {
	labelValue, ok := labels[labelName]
	if !ok {
		return Version{}, errors.Errorf("version label %s is missing", labelName)
	}

	v, err := semver.Parse(labelValue)
	if err != nil {
		return Version{}, errors.Wrapf(err, "version label %s is invalid: %s", labelName, labelValue)
	}

	return v, nil
}

func mustNewPRVersion(s string) semver.PRVersion {
	ver, err := semver.NewPRVersion(s)
	if err != nil {
		panic(`version: mustNewPRVersion(` + s + `): ` + err.Error())
	}

	return ver
}
