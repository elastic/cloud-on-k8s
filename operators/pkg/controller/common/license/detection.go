/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
)

func isLicenseType(secret corev1.Secret, licenseType EnterpriseLicenseType) bool {
	// is it a license at all?
	if secret.Labels[common.TypeLabelName] != Type {
		return false
	}
	// potentially not set before first reconcile attempt
	actualType, hasLabel := secret.Labels[LicenseLabelType]
	if hasLabel {
		return actualType == string(licenseType)
	}
	// user created license secret without data implies trial
	if len(secret.Data) == 0 && licenseType == LicenseTypeEnterpriseTrial {
		return true
	}
	// last resort parse the actual license data
	license, err := ParseEnterpriseLicense(secret.Data)
	if err != nil {
		return false
	}
	// XOR expected enterprise license type and trial type
	return (licenseType == LicenseTypeEnterprise || license.IsTrial()) && !(licenseType == LicenseTypeEnterprise && license.IsTrial())
}

// IsEnterpriseTrial returns true if the given secret is a wrapper for an Enterprise Trial license
func IsEnterpriseTrial(secret corev1.Secret) bool {
	return isLicenseType(secret, LicenseTypeEnterpriseTrial)
}

func IsEnterpriseLicense(secret corev1.Secret) bool {
	return isLicenseType(secret, LicenseTypeEnterprise)
}
