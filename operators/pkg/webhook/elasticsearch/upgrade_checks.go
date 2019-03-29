// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"

	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/driver"
	_ "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version/version6"
	_ "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/version/version7"
	"github.com/pkg/errors"
)

const (
	noDowngradesMsg          = "Downgrades are not supported"
	parseVersionErrMsg       = "Cannot parse Elasticsearch version"
	parseStoredVersionErrMsg = "Cannot parse current Elasticsearch version"
)

func nilChecks(current, proposed *estype.Elasticsearch) *ValidationResult {
	if proposed == nil {
		return &ValidationResult{Allowed: false, Error: errors.New("nothing to validate")}
	}
	if current == nil {
		// newly created cluster
		return &ValidationResult{Allowed: true}
	}
	return nil
}

func unsupportedVersion(v *version.Version) string {
	return fmt.Sprintf("unsupported version: %v", v)
}

func parseVersion(current, proposed *estype.Elasticsearch) (*version.Version, *version.Version, *ValidationResult) {
	proposedVersion, err := version.Parse(proposed.Spec.Version)
	if err != nil {
		return nil, nil, &ValidationResult{Error: err, Reason: parseVersionErrMsg}
	}
	currentVersion, err := version.Parse(current.Spec.Version)
	if err != nil {
		return nil, nil, &ValidationResult{Error: err, Reason: parseStoredVersionErrMsg}
	}
	return currentVersion, proposedVersion, nil
}

func noDowngrades(current, proposed *estype.Elasticsearch) ValidationResult {
	if result := nilChecks(current, proposed); result != nil {
		return *result
	}
	currentVersion, proposedVersion, invalid := parseVersion(current, proposed)
	if invalid != nil {
		return *invalid
	}

	if !proposedVersion.IsSameOrAfter(*currentVersion) {
		return ValidationResult{Allowed: false, Reason: noDowngradesMsg}
	}
	return OK

}

func validUpgradePath(current, proposed *estype.Elasticsearch) ValidationResult {
	if result := nilChecks(current, proposed); result != nil {
		return *result
	}
	currentVersion, proposedVersion, invalid := parseVersion(current, proposed)
	if invalid != nil {
		return *invalid
	}

	v := driver.SupportedVersions(*proposedVersion)
	if v == nil {
		return ValidationResult{Allowed: false, Reason: unsupportedVersion(proposedVersion)}
	}
	err := v.VerifySupportsExistingVersion(*currentVersion, fmt.Sprintf("%s/%s", current.Namespace, current.Name))
	if err != nil {
		return ValidationResult{Allowed: false, Reason: err.Error()}
	}
	return OK
}
