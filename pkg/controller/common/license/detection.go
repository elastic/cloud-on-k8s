// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

func isLicenseType(secret corev1.Secret, licenseType OperatorLicenseType) bool {
	// is it a license at all (but be lenient if user omitted label)?
	baseType, hasLabel := secret.Labels[commonv1.TypeLabelName]
	if hasLabel && baseType != Type {
		return false
	}
	// required to be set by user to detect license
	return secret.Labels[LicenseLabelType] == string(licenseType)
}

// IsEnterpriseTrial returns true if the given secret is a wrapper for an Enterprise Trial license
func IsEnterpriseTrial(secret corev1.Secret) bool {
	// we need to support legacy trial license secrets for backwards compatibility
	return isLicenseType(secret, LicenseTypeEnterpriseTrial) || isLicenseType(secret, LicenseTypeLegacyTrial)
}

// IsOperatorLicense returns true if the given secret is a wrapper for an operator license.
func IsOperatorLicense(secret corev1.Secret) bool {
	scope, hasLabel := secret.Labels[LicenseLabelScope]
	return hasLabel && scope == string(LicenseScopeOperator)
}
