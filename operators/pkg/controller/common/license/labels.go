// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// LicenseLabelName is a label pointing to the name of the source enterprise license.
	LicenseLabelName  = "license.k8s.elastic.co/name"
	LicenseLabelType  = "license.k8s.elastic.co/type"
	LicenseLabelState = "license.k8s.elastic.co/state"
	Type              = "license"
)

// LicenseType is the type of license a resource is describing.
type LicenseType string

const (
	LicenseTypeEnterprise LicenseType = "enterprise"
	LicenseTypeCluster    LicenseType = "cluster"
)

func LabelsForType(licenseType LicenseType) map[string]string {
	return map[string]string{
		common.TypeLabelName: Type,
		LicenseLabelType:     string(licenseType),
	}
}

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
