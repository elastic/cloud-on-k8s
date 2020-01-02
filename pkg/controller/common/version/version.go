// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package version

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Version is a parsed version
type Version struct {
	Major int
	Minor int
	Patch int
	Label string
}

// String formats the version into a string
func (v Version) String() string {
	vString := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Label != "" {
		vString += "-" + v.Label
	}
	return vString
}

var (
	// TooFewSegmentsErrorMessage is used as an error message when a version has too few dot-separated segments
	TooFewSegmentsErrorMessage = "version string has too few segments: %s"
	// TooManySegmentsErrorMessage is used as an error message when a version has too many dot-separated segments
	TooManySegmentsErrorMessage = "version string has too many segments: %s"
)

// Parse returns a parsed version of a string from the format {major}.{minor}.{patch}[-{label}]
func Parse(version string) (*Version, error) {
	segments := strings.SplitN(version, ".", 3)
	if len(segments) < 3 {
		return nil, errors.Errorf(TooFewSegmentsErrorMessage, version)
	}
	if len(segments) > 4 {
		return nil, errors.Errorf(TooManySegmentsErrorMessage, version)
	}

	major, err := strconv.Atoi(segments[0])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid major format. version: %s", version)
	}

	minor, err := strconv.Atoi(segments[1])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid minor format. version: %s", version)
	}

	patchSegments := strings.SplitN(segments[2], "-", 2)

	patch, err := strconv.Atoi(patchSegments[0])
	if err != nil {
		return nil, errors.Wrapf(err, "invalid patch format. version: %s", version)
	}

	label := ""
	if len(patchSegments) == 2 {
		label = patchSegments[1]
	}

	return &Version{Major: major, Minor: minor, Patch: patch, Label: label}, nil
}

// From creates a new version from the given major, minor, patch numbers.
func From(major, minor, patch int) Version {
	return Version{Major: major, Minor: minor, Patch: patch}
}

// MustParse is a variant of Parse that panics if the version is not valid
func MustParse(version string) Version {
	v, err := Parse(version)
	if err != nil {
		panic(err)
	}
	return *v
}

// IsSameOrAfter returns true if the receiver is the same version or newer than the argument. Labels are ignored.
func (v *Version) IsSameOrAfter(other Version) bool {
	return v.Major > other.Major ||
		(v.Major == other.Major && v.Minor > other.Minor) ||
		(v.Major == other.Major && v.Minor == other.Minor && v.Patch >= other.Patch)
}

// Min returns the minimum version in vs or nil.
func Min(vs []Version) *Version {
	sort.SliceStable(vs, func(i, j int) bool {
		return vs[j].IsSameOrAfter(vs[i])
	})
	var v *Version
	if len(vs) > 0 {
		v = &vs[0]
	}
	return v
}
