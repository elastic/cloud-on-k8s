// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
)

func compareConfigs(actual *settings.CanonicalConfig, expected *settings.CanonicalConfig) Comparison {
	// check for settings in actual that do not match expected
	diff := actual.Diff(expected, toIgnore)
	if len(diff) == 0 {
		return ComparisonMatch
	}

	var reasons []string
	for _, mismatch := range diff {
		reasons = append(reasons, fmt.Sprintf("Configuration setting mismatch: %s.", mismatch))
	}
	return ComparisonMismatch(reasons...)
}

var toIgnore = []string{
	settings.NodeName,
	settings.DiscoveryZenMinimumMasterNodes,
	settings.ClusterInitialMasterNodes,
	settings.NetworkPublishHost,
}
