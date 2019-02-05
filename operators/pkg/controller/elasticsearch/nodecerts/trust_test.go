package nodecerts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

func TestTrustRootConfig_Include(t *testing.T) {
	tests := []struct {
		name              string
		trustRootConfig   TrustRootConfig
		trustRestrictions v1alpha1.TrustRestrictions
		expected          TrustRootConfig
	}{
		{
			name:            "include new subject",
			trustRootConfig: TrustRootConfig{Trust: TrustConfig{SubjectName: []string{"foo"}}},
			trustRestrictions: v1alpha1.TrustRestrictions{
				Trust: v1alpha1.Trust{
					SubjectName: []string{"bar"},
				},
			},
			expected: TrustRootConfig{
				Trust: TrustConfig{SubjectName: []string{"foo", "bar"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.trustRootConfig.Include(tt.trustRestrictions)
			assert.Equal(t, tt.trustRootConfig, tt.expected)
		})
	}
}
