/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package elasticsearch

import (
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/pkg/errors"
)

const (
	noDowngradesMsg          = "Downgrades are not supported"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
)

func noDowngrades(current, proposed *estype.Elasticsearch) ValidationResult {
	if proposed == nil {
		return ValidationResult{Allowed: false, Error: errors.New("nothing to validate")}
	}
	if current == nil {
		// newly created cluster
		return ValidationResult{Allowed: true}
	}
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return ValidationResult{Error: err, Reason: parseVersionErrMsg}
	}
	currentVersion, err := version.Parse(current.Spec.Version)
	if err != nil {
		return ValidationResult{Error: err, Reason: parseStoredVersionErrMsg}
	}
	if !proposedVersion.IsSameOrAfter(*currentVersion) {
		return ValidationResult{Allowed: false, Reason: noDowngradesMsg}
	}
	return ValidationResult{Allowed: true}

}
