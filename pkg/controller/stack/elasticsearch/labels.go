package elasticsearch

import (
	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
)

const (
	// ClusterIDLabelName used to represent a cluster in k8s resources
	ClusterIDLabelName = "elasticsearch.stack.k8s.elastic.co/cluster-id"
	// HashLabelName used to represent a hash in k8s resources
	HashLabelName = "elasticsearch.stack.k8s.elastic.co/confighash"
	// TypeLabelName used to represent a resource type in k8s resources
	TypeLabelName = "stack.k8s.elastic.co/type"
	// TaintedLabelName used to represent a tainted resource in k8s resources
	TaintedLabelName = "elasticsearch.stack.k8s.elastic.co/tainted"
)

// NewLabels constructs a new set of labels from a Stack definition.
func NewLabels(s deploymentsv1alpha1.Stack, hash bool) map[string]string {
	var labels = map[string]string{
		ClusterIDLabelName: common.StackID(s),
		TypeLabelName:      "elasticsearch",
	}

	if hash {
		labels[HashLabelName] = BuildNewPodSpecParams(s).Hash()
	}

	return labels
}
