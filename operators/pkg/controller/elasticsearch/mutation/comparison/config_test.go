// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/require"
)

func Test_compareConfigs(t *testing.T) {
	var subCfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(`{"b": [1, 2, 3]}`), &subCfg))
	tests := []struct {
		name     string
		expected *settings.FlatConfig
		actual   *settings.FlatConfig
		want     Comparison
	}{
		{
			name: "same config",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
			}),
			want: ComparisonMatch,
		},
		{
			name: "different config item",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "eee",
			}),
			want: ComparisonMismatch("Configuration setting mismatch: c."),
		},
		{
			name: "one more item in expected",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
				"e": "f",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
			}),
			want: ComparisonMismatch("Configuration setting mismatch: e."),
		},
		{
			name: "one more item in actual",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a": "b",
				"c": "d",
				"e": "f",
			}),
			want: ComparisonMismatch("Configuration setting mismatch: e."),
		},
		{
			name: "some fields should be ignored",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a":                                     "b",
				settings.NodeName:                       "expected-node",
				settings.DiscoveryZenMinimumMasterNodes: 1,
				settings.ClusterInitialMasterNodes:      "1",
				settings.NetworkPublishHost:             "1.2.3.4",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a":                                     "b",
				settings.NodeName:                       "actual-node",
				settings.DiscoveryZenMinimumMasterNodes: 12,
				settings.ClusterInitialMasterNodes:      "3",
				settings.NetworkPublishHost:             "1.2.3.45",
			}),
			want: ComparisonMatch,
		},
		{
			name: "some fields should be ignored but should not prevent mismatch",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a":                                     "b",
				settings.NodeName:                       "expected-node",
				settings.DiscoveryZenMinimumMasterNodes: "1",
				settings.ClusterInitialMasterNodes:      "1",
				settings.NetworkPublishHost:             "1.2.3.4",
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a":                                     "mismatch",
				settings.NodeName:                       "actual-node",
				settings.DiscoveryZenMinimumMasterNodes: "12",
				settings.ClusterInitialMasterNodes:      "3",
				settings.NetworkPublishHost:             "1.2.3.45",
			}),
			want: ComparisonMismatch("Configuration setting mismatch: a."),
		},
		{
			name: "pure int config",
			expected: settings.MustFlatConfig(map[string]interface{}{
				"a": subCfg,
				"b": 2,
			}),
			actual: settings.MustFlatConfig(map[string]interface{}{
				"a": subCfg,
				"b": 3,
			}),
			want: ComparisonMismatch("Configuration setting mismatch: b."),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareConfigs(tt.actual, tt.expected); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compareConfigs() = %v, want %v", got, tt.want)
			}
		})
	}
}
