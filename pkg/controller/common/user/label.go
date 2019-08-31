// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package user

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// UserType is used to annotate a secret and described it as an external user.
	UserType = "user"
)

// NewLabelSelectorForElasticsearch returns a labels.Selector that matches the labels as constructed by
// NewLabels for the provided cluster name and of for a resource of type "user"
func NewLabelSelectorForElasticsearch(es v1alpha1.Elasticsearch) client.MatchingLabels {
	return client.MatchingLabels(map[string]string{
		label.ClusterNameLabelName: es.Name,
		common.TypeLabelName:       UserType,
	})
	// TODO sabo why does this not work? says it is missing GetAnnotations because it has a pointer receiver
	// return client.MatchingLabels(NewLabels(k8s.ExtractNamespacedName(es)))
}

// NewLabels constructs a new set of labels from an Elasticsearch cluster name for a resource of type "user".
func NewLabels(es types.NamespacedName) map[string]string {
	return map[string]string{
		label.ClusterNameLabelName: es.Name,
		common.TypeLabelName:       UserType,
	}
}

// NewToRequestsFuncFromClusterNameLabel creates a watch handler function that creates reconcile requests based on the
// the cluster name label if the resource is of type "user".
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
