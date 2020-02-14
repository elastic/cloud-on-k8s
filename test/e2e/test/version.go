// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"testing"
)

// Elastic Stack versions used in the E2E tests
var (
	// Minimum version for 6.8.x tested with the operator
	MinVersion68x = "6.8.5"
	// Minimum version for 7.x tested with the operator
	MinVersion7x = "7.1.1"
	// Current latest version for 7.x
	LatestVersion7x = "7.6.0" // version to synchronize with the latest release of the Elastic Stack
)

func SkipIfMinVersion68x(t *testing.T) {
	if Ctx().ElasticStackVersion == MinVersion68x {
		t.SkipNow()
	}
}

func SkipIfFrom7xTo7x(t *testing.T) {
	v := Ctx().ElasticStackVersion
	if v == MinVersion7x || v == LatestVersion7x {
		t.SkipNow()
	}
}
