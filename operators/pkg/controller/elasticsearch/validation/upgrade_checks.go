// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
)

const (
	noDowngradesMsg = "Downgrades are not supported"
)

func unsupportedVersion(v *version.Version) string {
	return fmt.Sprintf("unsupported version: %v", v)
}

func noDowngrades(ctx Context) Result {
	if ctx.isCreate() {
		return OK
	}
	if !ctx.Proposed.Version.IsSameOrAfter(ctx.Current.Version) {
		return Result{Allowed: false, Reason: noDowngradesMsg}
	}
	return OK
}

func validUpgradePath(ctx Context) Result {
	if ctx.isCreate() {
		return OK
	}

	v := driver.SupportedVersions(ctx.Proposed.Version)
	if v == nil {
		return Result{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}
	}
	err := v.Supports(ctx.Current.Version)
	if err != nil {
		return Result{Allowed: false, Reason: err.Error()}
	}
	return OK
}
