/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package validation

import "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"

// Validations are all registered Elasticsearch validations.
var Validations = []Validation{
	hasMaster,
	supportedVersion,
	noDowngrades,
	validUpgradePath,
}

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
