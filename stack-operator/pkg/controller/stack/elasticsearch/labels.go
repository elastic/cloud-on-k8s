package elasticsearch

import (
	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	// ClusterIDLabelName used to represent a cluster in k8s resources
	ClusterIDLabelName = "elasticsearch.stack.k8s.elastic.co/cluster-id"
	// TypeLabelName used to represent a resource type in k8s resources
	TypeLabelName = "stack.k8s.elastic.co/type"
	// Type represents the elasticsearch type
	Type = "elasticsearch"
	// TaintedLabelName used to represent a tainted resource in k8s resources
	TaintedLabelName = "elasticsearch.stack.k8s.elastic.co/tainted"
)

// TypeSelector is a selector on the the Elasticsearch type present in a Pod's labels
var TypeSelector = labels.Set(map[string]string{TypeLabelName: Type}).AsSelector()

// NewLabels constructs a new set of labels from a Stack definition.
func NewLabels(s deploymentsv1alpha1.Stack, hash bool) map[string]string {
	var labels = map[string]string{
		ClusterIDLabelName: common.StackID(s),
		TypeLabelName:      Type,
	}

	return labels
}

// NewLabelSelectorForStack returns a labels.Selector that matches the stack labels as constructed by NewLabels
func NewLabelSelectorForStack(s deploymentsv1alpha1.Stack) (labels.Selector, error) {
	req, err := labels.NewRequirement(
		ClusterIDLabelName,
		selection.Equals,
		[]string{common.StackID(s)},
	)
	if err != nil {
		return nil, err
	}

	sel := TypeSelector.DeepCopySelector().Add(*req)
	return sel, nil
}
