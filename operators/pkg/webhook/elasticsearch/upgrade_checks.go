// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	notMajorVersionUpgradeMsg = "Major version upgrades are currently not supported"
	parseVersionErrMsg        = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg  = "Cannot parse current Elasticsearch version"
)

func (v *Validation) canUpgrade(ctx context.Context, proposed estype.Elasticsearch) ValidationResult {
	var current estype.Elasticsearch
	err := v.client.Get(ctx, k8s.ExtractNamespacedName(&proposed), &current)
	if errors.IsNotFound(err) {
		return ValidationResult{Allowed: true} // not created yet
	}
	if err != nil {
		return ValidationResult{Error: err, Reason: "Cannot load current version of Elasticsearch resource"}
	}
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return ValidationResult{Error: err, Reason: parseVersionErrMsg}
	}
	currentVersion, err := version.Parse(current.Spec.Version)
	if err != nil {
		return ValidationResult{Error: err, Reason: parseStoredVersionErrMsg}
	}
	if currentVersion.Major != proposedVersion.Major {
		return ValidationResult{Allowed: false, Reason: notMajorVersionUpgradeMsg}
	}
	return ValidationResult{Allowed: true}
}
