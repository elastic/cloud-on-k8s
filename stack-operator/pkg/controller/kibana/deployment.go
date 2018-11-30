package kibana

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
)

func PseudoNamespacedResourceName(kb v1alpha1.Kibana) string {
	return common.Concat(kb.Name, "-kibana")
}
