// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/name"
)

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
	if len(ctx.Proposed.Elasticsearch.Name) > name.MaxElasticsearchNameLength {
		return Result{Allowed: false, Reason: fmt.Sprintf(nameTooLongErrMsg, name.MaxElasticsearchNameLength)}
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
	for _, t := range ctx.Proposed.Elasticsearch.Spec.Nodes {
		cfg, err := t.Config.Unpack()
		if err != nil {
			return Result{Reason: cfgInvalidMsg}
		}
		hasMaster = hasMaster || (cfg.Node.Master && t.NodeCount > 0)
	}
	if hasMaster {
		return OK
	}
	return Result{Reason: masterRequiredMsg}
}

func noBlacklistedSettings(ctx Context) Result {
	violations := make(map[int]map[string]struct{})
	for i, n := range ctx.Proposed.Elasticsearch.Spec.Nodes {
		config, err := settings.NewCanonicalConfigFrom(n.Config)
		if err != nil {
			violations[i] = map[string]struct{}{
				cfgInvalidMsg: {},
			}
			continue
		}
		forbidden := config.HasPrefix(settings.Blacklist)
		// remove duplicates
		set := make(map[string]struct{})
		for _, k := range forbidden {
			set[k] = struct{}{}
		}
		if len(forbidden) > 0 {
			violations[i] = set
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
		var sep2 string
		for k, _ := range v {
			sb.WriteString(sep2)
			sb.WriteString(k)
			sep2 = ", "
		}
		sep = "; "
	}
	sb.WriteString(" is not user configurable")
	return Result{
		Allowed: false,
		Reason:  sb.String(),
	}
}
