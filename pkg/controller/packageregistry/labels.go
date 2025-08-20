// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/epr/v1alpha1"
)

const (
	// NameLabelName used to represent a EPR in k8s resources
	NameLabelName = "packageregistry.k8s.elastic.co/name"

	// versionLabelName used to propagate EPR version from the spec to the pods
	VersionLabelName = "packageregistry.k8s.elastic.co/version"
	// PackageRegistryNamespaceLabelName used to represent a Package Registry in k8s resources.
	PackageRegistryNamespaceLabelName = "packageregistry.k8s.elastic.co/namespace"

	// Type represents the MapsServer type
	Type = "packageregistry"
)

func versionLabels(epr eprv1alpha1.ElasticPackageRegistry) map[string]string {
	return map[string]string{
		VersionLabelName: epr.Spec.Version,
	}
}
