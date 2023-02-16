// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
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

type LicenseScope string //nolint:revive

const (
	LicenseScopeOperator      LicenseScope = "operator"
	LicenseScopeElasticsearch LicenseScope = "elasticsearch"
)

// LabelsForOperatorScope creates a map of labels for operator scope with licence type set to the given value.
func LabelsForOperatorScope(t OperatorLicenseType) map[string]string {
	return map[string]string{
		commonv1.TypeLabelName: Type,
		LicenseLabelScope:      string(LicenseScopeOperator),
		LicenseLabelType:       string(t),
	}
}

func NewLicenseByScopeSelector(scope LicenseScope) client.MatchingLabels {
	return map[string]string{
		LicenseLabelScope: string(scope),
	}
}
