package v1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

// GetElasticLabels will return the common Elastic assigned labels for EnterpriseSearch.
func (es *EnterpriseSearch) GetElasticLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:                 "enterprise-search",
		"enterprisesearch.k8s.elastic.co/name": es.Name,
	}
}
