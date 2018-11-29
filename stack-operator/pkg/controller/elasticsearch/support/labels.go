package support

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	// ClusterNameLabelName used to represent a cluster in k8s resources
	ClusterNameLabelName = "elasticsearch.k8s.elastic.co/cluster-name"

	// TODO: move to another package
	// TypeLabelName used to represent a resource type in k8s resources
	TypeLabelName = "common.k8s.elastic.co/type"
	// Type represents the elasticsearch type
	Type = "elasticsearch"
)

// TypeSelector is a selector on the the Elasticsearch type present in a Pod's labels
var TypeSelector = labels.Set(map[string]string{TypeLabelName: Type}).AsSelector()

func PseudoNamespacedResourceName(es v1alpha1.ElasticsearchCluster) string {
	return common.Concat(es.Name, "-elasticsearch")
}

// NewLabels constructs a new set of labels from an Elasticsearch definition.
func NewLabels(es v1alpha1.ElasticsearchCluster) map[string]string {
	var labels = map[string]string{
		ClusterNameLabelName: es.Name,
		TypeLabelName:        Type,
	}

	return labels
}

// NewLabelSelectorForElasticsearch returns a labels.Selector that matches the labels as constructed by NewLabels
func NewLabelSelectorForElasticsearch(es v1alpha1.ElasticsearchCluster) (labels.Selector, error) {
	req, err := labels.NewRequirement(
		ClusterNameLabelName,
		selection.Equals,
		[]string{es.Name},
	)
	if err != nil {
		return nil, err
	}

	sel := TypeSelector.DeepCopySelector().Add(*req)
	return sel, nil
}
