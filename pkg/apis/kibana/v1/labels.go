package v1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

// GetElasticLabels will return the common Elastic assigned labels for Kibana.
func (k *Kibana) GetElasticLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:       "kibana",
		"kibana.k8s.elastic.co/name": k.Name,
	}
}
