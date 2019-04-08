// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
)

const maxNameLength = 36

// Validations are all registered Elasticsearch validations.
var Validations = []Validation{
	nameLength,
	hasMaster,
	supportedVersion,
	noDowngrades,
	validUpgradePath,
}

// nameLength checks the length of the Elasticsearch name.
func nameLength(ctx Context) Result {
	if len(ctx.Proposed.Elasticsearch.Name) > maxNameLength {
		return Result{Allowed: false, Reason: fmt.Sprintf(nameTooLongErrMsg, maxNameLength)}
	}
	return OK
}

// supportedVersion checks if the version is supported.
func supportedVersion(ctx Context) Result {
	if v := driver.SupportedVersions(ctx.Proposed.Version); v == nil {
		return Result{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}
	}
	return OK
}

// hasMaster checks if the given Elasticsearch cluster has at least one master node.
func hasMaster(ctx Context) Result {
	var hasMaster bool
	for _, t := range ctx.Proposed.Elasticsearch.Spec.Topology {
		hasMaster = hasMaster || (t.NodeTypes.Master && t.NodeCount > 0)
	}
	if hasMaster {
		return OK
	}
	return Result{Reason: masterRequiredMsg}
}
