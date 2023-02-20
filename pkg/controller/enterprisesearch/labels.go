// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
)

const (
	// Type represents the Enterprise Search type.
	Type = "enterprise-search"
	// EnterpriseSearchNameLabelName used to represent an EnterpriseSearch in k8s resources.
	EnterpriseSearchNameLabelName = "enterprisesearch.k8s.elastic.co/name"
	// EnterpriseSearchNamespaceLabelName used to represent an EnterpriseSearch in k8s resources.
	EnterpriseSearchNamespaceLabelName = "enterprisesearch.k8s.elastic.co/namespace"
	// VersionLabelName is a label used to track the version of an Enterprise Search Pod.
	VersionLabelName = "enterprisesearch.k8s.elastic.co/version"
)

func VersionLabels(ent entv1.EnterpriseSearch) map[string]string {
	return map[string]string{
		VersionLabelName: ent.Spec.Version,
	}
}
