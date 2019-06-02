// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// LicenseLabelName is a label pointing to the name of the source enterprise license.
	LicenseLabelName  = "license.k8s.elastic.co/name"
	LicenseLabelType  = "license.k8s.elastic.co/type"
	LicenseLabelState = "license.k8s.elastic.co/state"
)

// NewLicenseByNameSelector is a list selector to filter by a label containing the license name.
func NewLicenseByNameSelector(licenseName string) labels.Selector {
	return labels.Set(map[string]string{
		LicenseLabelName: licenseName,
	}).AsSelector()
}

func NewLicenseByTypeSelector(licenseType string) labels.Selector {
	return labels.Set(map[string]string{
		LicenseLabelType: licenseType,
	}).AsSelector()
}
