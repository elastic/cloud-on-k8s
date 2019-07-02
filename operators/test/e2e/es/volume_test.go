// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
)

// TestVolumeEmptyDir tests a manual override of the default persistent storage with emptyDir.
func TestVolumeEmptyDir(t *testing.T) {
	b := elasticsearch.NewBuilder("test-es-explicit-empty-dir").
		WithESMasterNodes(1, elasticsearch.DefaultResources).
		WithEmptyDirVolumes()

	// volume type will be checked in creation steps
	test.Run(t, test.EmptySteps, b)
}
