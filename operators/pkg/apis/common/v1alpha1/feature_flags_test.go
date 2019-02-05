package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testUnknownFeatureFlag is a feature flag that is unknown to the FeatureFlags.Get method
var testUnknownFeatureFlag = FeatureFlag("test-unknown")

func TestFeatureFlags(t *testing.T) {
	t.Run("when empty", func(t *testing.T) {
		empty := FeatureFlags(map[FeatureFlag]FeatureFlagState{})

		assert.Equal(t, empty.Get(FeatureFlagExample), FeatureFlagExampleDefaultState)
	})

	t.Run("when non-empty", func(t *testing.T) {
		type test struct {
			key      FeatureFlag
			expected FeatureFlagState
		}
		tests := []struct {
			flags FeatureFlags
			tests []test
		}{
			{
				flags: FeatureFlags(map[FeatureFlag]FeatureFlagState{
					FeatureFlagExample: {Enabled: true},
				}),
				tests: []test{
					{key: FeatureFlagExample, expected: FeatureFlagState{Enabled: true}},
					{key: testUnknownFeatureFlag, expected: FeatureFlagExampleDefaultState},
				},
			}, {
				flags: FeatureFlags(map[FeatureFlag]FeatureFlagState{
					FeatureFlagExample: {Enabled: false},
				}),
				tests: []test{
					{key: FeatureFlagExample, expected: FeatureFlagState{Enabled: false}},
					{key: testUnknownFeatureFlag, expected: FeatureFlagExampleDefaultState},
				},
			},
		}

		for _, test := range tests {
			for _, validate := range test.tests {
				assert.Equal(t, test.flags.Get(validate.key), validate.expected)
			}
		}
	})
}
