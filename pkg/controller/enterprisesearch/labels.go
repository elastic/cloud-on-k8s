// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import "github.com/elastic/cloud-on-k8s/pkg/controller/common"

const (
	// EnterpriseSearchNameLabelName used to represent an EnterpriseSearch in k8s resources
	EnterpriseSearchNameLabelName = "enterprisesearch.k8s.elastic.co/name"
	// Type represents the Enterprise Search type
	Type = "enterprise-search"
)

// NewLabels constructs a new set of labels for an Enterprise Search pod
func NewLabels(entsName string) map[string]string {
	return map[string]string{
		EnterpriseSearchNameLabelName: entsName,
		common.TypeLabelName:          Type,
	}
}
