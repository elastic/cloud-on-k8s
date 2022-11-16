// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"strings"
)

const Annotation = "eck.k8s.elastic.co/license"

// HasRequestedLicenseLevel returns true if the operator license level matches the level expressed in the map of annotations.
func HasRequestedLicenseLevel(ctx context.Context, annotations map[string]string, checker Checker) (bool, error) {
	requestedLevel, exists := annotations[Annotation]
	if !exists {
		return true, nil // no annotation no restrictions
	}
	requestedLicenseType := OperatorLicenseType(strings.ToLower(requestedLevel))
	if _, exists := OperatorLicenseTypeOrder[requestedLicenseType]; !exists {
		return true, nil // be lenient in case of misspelled or incorrect license names
	}
	if requestedLicenseType == LicenseTypeBasic {
		return true, nil // basic is always allowed
	}
	return checker.EnterpriseFeaturesEnabled(ctx) // if enterprise features are on, everything is allowed.
}
