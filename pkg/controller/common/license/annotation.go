// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"strings"
)

const Annotation = "eck.k8s.elastic.co/license"

func HasRequestedLicenseLevel(annotations map[string]string, checker Checker) (bool, error) {
	requestedLevel, exists := annotations[Annotation]
	if !exists {
		return true, nil // no annotation no restrictions
	}
	if OperatorLicenseType(strings.ToLower(requestedLevel)) == LicenseTypeBasic {
		return true, nil // basic is always allowed
	}
	return checker.EnterpriseFeaturesEnabled() // if enterprise features are on, everything is allowed.
}
