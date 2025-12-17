// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package label

const (
	// NameLabelName used to represent a EPR in k8s resources
	NameLabelName = "packageregistry.k8s.elastic.co/name"

	// VersionLabelName used to propagate EPR version from the spec to the pods
	VersionLabelName = "packageregistry.k8s.elastic.co/version"
	// PackageRegistryNamespaceLabelName used to represent a Package Registry in k8s resources.
	PackageRegistryNamespaceLabelName = "packageregistry.k8s.elastic.co/namespace"

	// Type represents the PackageRegistry type
	Type = "package-registry"
)
