package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

// GetElasticLabels will return the common Elastic assigned labels for EnterprisMapsServer.
func (m *ElasticMapsServer) GetElasticLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:     "maps",
		"maps.k8s.elastic.co/name": m.Name,
	}
}
