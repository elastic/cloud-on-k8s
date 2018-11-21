package v1alpha1

// FeatureFlags is a collection of feature flags and their associated state
type FeatureFlags map[FeatureFlag]FeatureFlagState

// Get returns a FeatureFlag from the map, or its default state if it's not set.
func (f FeatureFlags) Get(flag FeatureFlag) FeatureFlagState {
	if state, ok := f[flag]; ok {
		return state
	}

	switch flag {
	case FeatureFlagNodeCertificates:
		return FeatureFlagNodeCertificatesDefaultState
	}

	return FeatureFlagState{}
}

// FeatureFlag is a unique identifier used for feature flags
type FeatureFlag string

const (
	// FeatureFlagNodeCertificates configures whether we configure tls between nodes. The fact that it's called internal
	// is a bit of a misnomer, as it also includes encryption for HTTP as well.
	FeatureFlagNodeCertificates = FeatureFlag("nodeCertificates")
)

var (
	// FeatureFlagNodeCertificatesDefaultState is the default state for the FeatureFlagNodeCertificates feature flag.
	FeatureFlagNodeCertificatesDefaultState = FeatureFlagState{Enabled: false}
)

// FeatureFlagState contains the configured state of a FeatureFlag
type FeatureFlagState struct {
	// Enabled enables this feature flag.
	Enabled bool `json:"enabled,omitempty"`
}
