// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
)

const (
	// Type represents the Enterprise Search type.
	Type = "enterprise-search"
	// EnterpriseSearchNameLabelName used to represent an EnterpriseSearch in k8s resources.
	EnterpriseSearchNameLabelName = "enterprisesearch.k8s.elastic.co/name"
	// VersionLabelName is a label used to track the version of an Enterprise Search Pod.
	VersionLabelName = "enterprisesearch.k8s.elastic.co/version"
)

// Labels returns labels that identify the given Enterprise Search resource
func Labels(entsName string) map[string]string {
	return map[string]string{
		EnterpriseSearchNameLabelName: entsName,
		common.TypeLabelName:          Type,
	}
}

func VersionLabels(ents entsv1beta1.EnterpriseSearch) map[string]string {
	return map[string]string{
		VersionLabelName: ents.Spec.Version,
	}
}
