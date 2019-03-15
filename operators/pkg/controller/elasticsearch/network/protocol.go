// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package network

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
)

// ProtocolForCluster returns the protocol (http or https) to be used
// for requesting the Elasticsearch cluster.
func ProtocolForCluster(es v1alpha1.Elasticsearch) string {
	return ProtocolForLicense(es.Spec.GetLicenseType())
}

// ProtocolForLicense returns the protocol (http or https) to be used
// for requesting the Elasticsearch cluster.
func ProtocolForLicense(licenseType v1alpha1.LicenseType) string {
	if licenseType == v1alpha1.LicenseTypeBasic {
		return "http"
	}
	return "https"
}
