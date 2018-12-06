package support

import (
	"strconv"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
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

	// NodeTypesDataLabelName is a label set to true on nodes with the master role
	NodeTypesMasterLabelName TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-master"
	// NodeTypesDataLabelName is a label set to true on nodes with the data role
	NodeTypesDataLabelName TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data"
	// NodeTypesIngestLabelName is a label set to true on nodes with the ingest role
	NodeTypesIngestLabelName TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ingest"
	// NodeTypesMLLabelName is a label set to true on nodes with the ml role
	NodeTypesMLLabelName TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ml"
)

// TrueFalseLabel is a label that has a true/false value.
type TrueFalseLabel string

// Set sets the given value of this label in the provided map
func (l TrueFalseLabel) Set(value bool, labels map[string]string) {
	labels[string(l)] = strconv.FormatBool(value)
}

// HasValue returns true if this label has the specified value in the provided map
func (l TrueFalseLabel) HasValue(value bool, labels map[string]string) bool {
	return labels[string(l)] == strconv.FormatBool(value)
}

// AsMap is a convenience method to create a map with this label set to a specific value
func (l TrueFalseLabel) AsMap(value bool) map[string]string {
	return map[string]string{
		string(l): strconv.FormatBool(value),
	}
}

// TypeSelector is a selector on the the Elasticsearch type present in a Pod's labels
var TypeSelector = labels.Set(map[string]string{TypeLabelName: Type}).AsSelector()

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
