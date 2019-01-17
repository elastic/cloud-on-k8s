package label

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

const (
	// ClusterNameLabelName used to represent a cluster in k8s resources
	ClusterNameLabelName = "elasticsearch.k8s.elastic.co/cluster-name"

	// Type represents the elasticsearch type
	Type = "elasticsearch"

	// NodeTypesMasterLabelName is a label set to true on nodes with the master role
	NodeTypesMasterLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-master"
	// NodeTypesDataLabelName is a label set to true on nodes with the data role
	NodeTypesDataLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-data"
	// NodeTypesIngestLabelName is a label set to true on nodes with the ingest role
	NodeTypesIngestLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ingest"
	// NodeTypesMLLabelName is a label set to true on nodes with the ml role
	NodeTypesMLLabelName common.TrueFalseLabel = "elasticsearch.k8s.elastic.co/node-ml"
)

// IsMasterNode returns true if the pod has the master node label
func IsMasterNode(pod corev1.Pod) bool {
	return NodeTypesMasterLabelName.HasValue(true, pod.Labels)
}

// IsDataNode returns true if the pod has the data node label
func IsDataNode(pod corev1.Pod) bool {
	return NodeTypesDataLabelName.HasValue(true, pod.Labels)
}

// TypeSelector is a selector on the Elasticsearch type present in a Pod's labels
var TypeSelector = labels.Set(map[string]string{common.TypeLabelName: Type}).AsSelector()

// NewLabels constructs a new set of labels from an Elasticsearch definition.
func NewLabels(es v1alpha1.ElasticsearchCluster) map[string]string {
	var labels = map[string]string{
		ClusterNameLabelName: es.Name,
		common.TypeLabelName: Type,
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
