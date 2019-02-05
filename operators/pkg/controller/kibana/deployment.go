package kibana

import (
	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/stack-operator/pkg/utils/stringsutil"
)

func PseudoNamespacedResourceName(kb v1alpha1.Kibana) string {
	return stringsutil.Concat(kb.Name, "-kibana")
}
