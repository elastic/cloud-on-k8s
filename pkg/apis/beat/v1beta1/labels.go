package v1beta1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

// GetElasticLabels will return the common Elastic assigned labels for the Beat.
func (b *Beat) GetElasticLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:     "beat",
		"beat.k8s.elastic.co/name": b.Name,
	}
}
