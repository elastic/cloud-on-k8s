// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package validation

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/validation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/driver"
)

const (
	noDowngradesMsg = "Downgrades are not supported"
)

func unsupportedVersion(v *version.Version) string {
	return fmt.Sprintf("unsupported version: %v", v)
}

func unsupportedUpgradePath(v1, v2 version.Version) string {
	return fmt.Sprintf("unsupported version upgrade from %v to %v", v1, v2)
}

func noDowngrades(ctx Context) validation.Result {
	if ctx.isCreate() {
		return validation.OK
	}
	if !ctx.Proposed.Version.IsSameOrAfter(ctx.Current.Version) {
		return validation.Result{Allowed: false, Reason: noDowngradesMsg}
	}
	return validation.OK
}

func validUpgradePath(ctx Context) validation.Result {
	if ctx.isCreate() {
		return validation.OK
	}

	v := driver.SupportedVersions(ctx.Proposed.Version)
	if v == nil {
		return validation.Result{Allowed: false, Reason: unsupportedVersion(&ctx.Proposed.Version)}
	}
	err := v.Supports(ctx.Current.Version)
	if err != nil {
		return validation.Result{
			Allowed: false,
			Reason:  unsupportedUpgradePath(ctx.Current.Version, ctx.Proposed.Version),
		}
	}
	return validation.OK
}
