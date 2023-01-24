package v1alpha1

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

// GetElasticLabels will return the common Elastic assigned labels for the Elastic Agent.
func (a *Agent) GetElasticLabels() map[string]string {
	return map[string]string{
		commonv1.TypeLabelName:      "agent",
		"agent.k8s.elastic.co/name": a.Name,
	}
}
