// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"fmt"
	"strconv"
)

// RBACOnRefsMode is the enforcement level for cross-namespace reference access checks.
type RBACOnRefsMode string

const (
	RBACOnRefsModeOff    RBACOnRefsMode = "false"
	RBACOnRefsModeTrue   RBACOnRefsMode = "true"
	RBACOnRefsModeLegacy RBACOnRefsMode = "legacy"
	RBACOnRefsModeAll    RBACOnRefsMode = "all"
)

// ParseRBACOnRefsMode validates and parses the flag string value.
// The canonical values are: false, true, legacy, all.
// For backward compatibility with the former boolean flag, truthy values accepted by
// strconv.ParseBool (e.g. "1", "t", "T", "TRUE") map to RBACOnRefsModeTrue and falsy
// values (e.g. "0", "f", "F", "FALSE") map to RBACOnRefsModeOff.
func ParseRBACOnRefsMode(v string) (RBACOnRefsMode, error) {
	switch RBACOnRefsMode(v) {
	case RBACOnRefsModeOff, RBACOnRefsModeTrue, RBACOnRefsModeLegacy, RBACOnRefsModeAll:
		return RBACOnRefsMode(v), nil
	default:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return RBACOnRefsModeOff, fmt.Errorf("invalid value %q, expected one of: false, true, legacy, all", v)
		}
		if b {
			return RBACOnRefsModeTrue, nil
		}
		return RBACOnRefsModeOff, nil
	}
}

// EnforcementEnabled returns true when any RBAC check is active.
func (m RBACOnRefsMode) EnforcementEnabled() bool {
	switch m {
	case RBACOnRefsModeTrue, RBACOnRefsModeLegacy, RBACOnRefsModeAll:
		return true
	default:
		return false
	}
}

// EnforcementAllAssociations returns true when RBAC is enforced for all associations.
func (m RBACOnRefsMode) EnforcementAllAssociations() bool {
	return m == RBACOnRefsModeAll
}

// StartupMessage returns a non-empty string for modes that require operator attention
// at startup, or an empty string if the mode is silent.
func (m RBACOnRefsMode) StartupMessage() string {
	switch m {
	case RBACOnRefsModeAll:
		return "RBAC enforcement is active for all cross-namespace associations: any association whose " +
			"service account lacks access to the referenced resource will be blocked."
	case RBACOnRefsModeLegacy:
		return "Warning: --enforce-rbac-on-refs=legacy is deprecated and will be removed in a future release. " +
			"It is currently identical to --enforce-rbac-on-refs=true: RBAC is checked for all associations, " +
			"but only direct or transitive Elasticsearch associations are enforced; for all other associations " +
			"warnings are emitted instead. Review the emitted warnings and update the relevant service account " +
			"RBAC rules accordingly, so that your setup is ready when the legacy mode is removed and RBAC is " +
			"enforced on all associations."
	case RBACOnRefsModeTrue:
		return "The behaviour of --enforce-rbac-on-refs=true is subject to change in a future release. " +
			"Currently only direct or transitive Elasticsearch associations are blocked on RBAC denial; all other " +
			"associations emit a warning but are not blocked. In a future release this value will become an alias " +
			"for --enforce-rbac-on-refs=all, which enforces RBAC on all associations. Review the warnings emitted " +
			"by the operator and update the relevant service account RBAC rules so that your setup is ready for " +
			"full enforcement."
	default:
		return ""
	}
}
