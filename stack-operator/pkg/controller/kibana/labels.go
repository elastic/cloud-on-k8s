package kibana

import "github.com/elastic/stack-operators/stack-operator/pkg/controller/common"

const (
	// KibanaNameLabelName used to represent a Kibana in k8s resources
	KibanaNameLabelName = "kibana.k8s.elastic.co/name"
	// Type represents the elasticsearch type
	Type = "elasticsearch"
)

// NewLabels constructs a new set of labels for a Kibana pod
func NewLabels(kibanaName string) map[string]string {
	return map[string]string{
		KibanaNameLabelName:  kibanaName,
		common.TypeLabelName: Type,
	}
}
