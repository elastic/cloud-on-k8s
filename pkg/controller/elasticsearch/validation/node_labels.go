// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	commonnodelabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels"
)

// NodeLabels is an alias for the shared exposed-node-labels policy. It is kept here so that
// existing callers can continue to import esvalidation.NodeLabels.
type NodeLabels = commonnodelabels.NodeLabels

// NewExposedNodeLabels delegates to the shared exposed-node-labels constructor.
func NewExposedNodeLabels(exposedNodeLabels []string) (NodeLabels, error) {
	return commonnodelabels.NewExposedNodeLabels(exposedNodeLabels)
}
