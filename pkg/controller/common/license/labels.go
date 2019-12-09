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
	LicenseLabelScope        = "license.k8s.elastic.co/scope"
	Type                     = "license"
	EULAAnnotation           = "elastic.co/eula"
	EULAAcceptedValue        = "accepted"
	LicenseInvalidAnnotation = "license.k8s.elastic.co/invalid"
)

type LicenseScope string

const (
	LicenseScopeOperator      LicenseScope = "operator"
	LicenseScopeElasticsearch LicenseScope = "elasticsearch"
)

// LabelsForOperatorScope creates a map of labels for operator scope with licence type set to the given value.
func LabelsForOperatorScope(t OperatorLicenseType) map[string]string {
	return map[string]string{
		common.TypeLabelName: Type,
		LicenseLabelScope:    string(LicenseScopeOperator),
		LicenseLabelType:     string(t),
	}
}

func NewLicenseByScopeSelector(scope LicenseScope) client.MatchingLabels {
	return map[string]string{
		LicenseLabelScope: string(scope),
	}
}
