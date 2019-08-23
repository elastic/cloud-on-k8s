// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

// FeatureFlags is a collection of feature flags and their associated state
type FeatureFlags map[FeatureFlag]FeatureFlagState

// Get returns a FeatureFlag from the map, or its default state if it's not set.
func (f FeatureFlags) Get(flag FeatureFlag) FeatureFlagState {
	if state, ok := f[flag]; ok {
		return state
	}

	if flag == FeatureFlagExample {
		return FeatureFlagExampleDefaultState
	}

	return FeatureFlagState{}
}

// FeatureFlag is a unique identifier used for feature flags
type FeatureFlag string

const (
	// FeatureFlagExample is a placeholder example feature flag.
	FeatureFlagExample = FeatureFlag("example")
)

var (
	// FeatureFlagExampleDefaultState is the default state for the FeatureFlagExample feature flag.
	FeatureFlagExampleDefaultState = FeatureFlagState{Enabled: false}
)

// FeatureFlagState contains the configured state of a FeatureFlag
type FeatureFlagState struct {
	// Enabled enables this feature flag.
	Enabled bool `json:"enabled,omitempty"`
}
