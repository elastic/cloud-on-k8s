// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"strconv"
	"strings"

	"github.com/elastic/go-ucfg"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
)

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
	for _, t := range ctx.Proposed.Elasticsearch.Spec.Nodes {
		hasMaster = hasMaster || (t.Config.IsMaster() && t.NodeCount > 0)
	}
	if hasMaster {
		return OK
	}
	return Result{Reason: masterRequiredMsg}
}

func noBlacklistedSettings(ctx Context) Result {
	violations := make(map[int]string)
	for i, n := range ctx.Proposed.Elasticsearch.Spec.Nodes {
		config, err := n.Config.Canonicalize()
		if err != nil {
			violations[i] = "[config invalid]"
			continue
		}
		keys := config.FlattenedKeys(ucfg.PathSep("."))
		for _, s := range settings.Blacklist {
			for _, k := range keys {
				if strings.HasPrefix(k, s) {
					violations[i] = s
				}
			}
		}
	}
	if len(violations) == 0 {
		return OK
	}
	var sb strings.Builder
	var sep string
	for n, v := range violations {
		sb.WriteString(sep)
		sb.WriteString("node[")
		sb.WriteString(strconv.FormatInt(int64(n), 10))
		sb.WriteString("]: ")
		sb.WriteString(v)
		sep = ", "
	}
	sb.WriteString(" is not user configurable")
	return Result{
		Allowed: false,
		Reason:  sb.String(),
	}
}
