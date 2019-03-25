// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
)

func compareConfigs(actual settings.FlatConfig, expected settings.FlatConfig) Comparison {
	// check for settings in actual that do not match expected
	for k, v := range actual {
		if ignoreFieldDuringComparison(k) {
			continue
		}
		expectedValue, exists := expected[k]
		if !exists || (expectedValue != v) {
			return ComparisonMismatch(fmt.Sprintf("Configuration setting mismatch: %s.", k))
		}
	}
	// check for settings in expected that don't exist in actual
	for k := range expected {
		if ignoreFieldDuringComparison(k) {
			continue
		}
		_, exists := actual[k]
		if !exists {
			return ComparisonMismatch(fmt.Sprintf("Configuration setting mismatch: %s.", k))
		}
	}
	return ComparisonMatch
}

// ignoreFieldDuringComparison returns true if the given configuration field should be
// ignored when pods are compared to expected pod specs
func ignoreFieldDuringComparison(field string) bool {
	switch field {
	case
		settings.NodeName,
		settings.DiscoveryZenMinimumMasterNodes,
		settings.ClusterInitialMasterNodes,
		settings.NetworkPublishHost:

		return true

	default:
		return false
	}
}
