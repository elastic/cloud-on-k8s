// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
)

func isLicenseType(secret corev1.Secret, licenseType EnterpriseLicenseType) bool {
	// is it a license at all (but be lenient if user omitted label)?
	baseType, hasLabel := secret.Labels[common.TypeLabelName]
	if hasLabel && baseType != Type {
		return false
	}
	// required to be set by user to detect license
	return secret.Labels[LicenseLabelType] == string(licenseType)
}

// IsEnterpriseTrial returns true if the given secret is a wrapper for an Enterprise Trial license
func IsEnterpriseTrial(secret corev1.Secret) bool {
	return isLicenseType(secret, LicenseTypeEnterpriseTrial)
}

func IsEnterpriseLicense(secret corev1.Secret) bool {
	return isLicenseType(secret, LicenseTypeEnterprise)
}
