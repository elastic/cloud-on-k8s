// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package network

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
)

// ProtocolForCluster returns the protocol (http or https) to be used
// for requesting the Elasticsearch cluster, according to its expected spec.
func ProtocolForCluster(es v1alpha1.Elasticsearch) string {
	return ProtocolForLicense(es.Spec.GetLicenseType())
}

// ProtocolForESPods inspects the given pods to return the protocol (http or https)
// that should be used to request the cluster.
func ProtocolForESPods(pods pod.PodsWithConfig) string {
	// default to https, unless at least one pod is configured for http
	for _, p := range pods {
		license := v1alpha1.LicenseType(p.Config[settings.XPackLicenseSelfGeneratedType])
		if ProtocolForLicense(license) == "http" {
			return "http"
		}
	}
	return "https"
}

// ProtocolForLicense returns the protocol (http or https) to be used
// for requesting the Elasticsearch cluster.
func ProtocolForLicense(licenseType v1alpha1.LicenseType) string {
	if licenseType == v1alpha1.LicenseTypeBasic {
		return "http"
	}
	return "https"
}
