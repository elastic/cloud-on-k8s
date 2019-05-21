package user

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	UserType = "user"
)

// NewLabelSelectorForElasticsearch returns a labels.Selector that matches the labels as constructed by
// NewLabels for the provided cluster name.
func NewLabelSelectorForElasticsearch(es v1alpha1.Elasticsearch) labels.Selector {
	return labels.SelectorFromSet(
		map[string]string{
			label.ClusterNameLabelName: es.Name,
			common.TypeLabelName:       UserType,
		})
}

// NewLabels constructs a new set of labels from an Elasticsearch definition.
func NewLabels(es types.NamespacedName) map[string]string {
	return map[string]string{
		label.ClusterNameLabelName: es.Name,
		common.TypeLabelName:       UserType,
	}
}

// NewToRequestsFuncFromClusterNameLabel creates a watch handler function that creates reconcile requests based on the
// the cluster name label on the watched resource.
func NewToRequestsFuncFromClusterNameLabel() handler.ToRequestsFunc {
	return handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
		labels := obj.Meta.GetLabels()
		if labelType, ok := labels[common.TypeLabelName]; !ok || labelType != UserType {
			return []reconcile.Request{}
		}

		if clusterName, ok := labels[label.ClusterNameLabelName]; ok {
			// we don't need to special case the handling of this label to support in-place changes to its value
			// as controller-runtime will ask this func to map both the old and the new resources on updates.
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{Namespace: obj.Meta.GetNamespace(), Name: clusterName}},
			}
		}
		return nil
	})
}
