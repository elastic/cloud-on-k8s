// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LicenseLabelName is a label pointing to the name of the source enterprise license.
	LicenseLabelName         = "license.k8s.elastic.co/name"
	LicenseLabelType         = "license.k8s.elastic.co/type"
	Type                     = "license"
	EULAAnnotation           = "elastic.co/eula"
	EULAAcceptedValue        = "accepted"
	LicenseInvalidAnnotation = "license.k8s.elastic.co/invalid"
)

// LicenseType is the type of license a resource is describing.
type LicenseType string

const (
	LicenseLabelEnterprise    LicenseType = "enterprise"
	LicenseLabelElasticsearch LicenseType = "elasticsearch"
)

// LabelsForType creates a map of labels for the given type of either enterprise or Elasticsearch license.
func LabelsForType(licenseType LicenseType) map[string]string {
	return map[string]string{
		common.TypeLabelName: Type,
		LicenseLabelType:     string(licenseType),
	}
}

// NewLicenseByNameSelector is a list selector to filter by a label containing the license name.
func NewLicenseByNameSelector(licenseName string) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{
		LicenseLabelName: licenseName,
	})
}

func NewLicenseByTypeSelector(licenseType string) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{
		LicenseLabelType: licenseType,
	})
}
