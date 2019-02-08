// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func listAffectedLicenses(c k8s.Client, s *runtime.Scheme, license types.NamespacedName) ([]reconcile.Request, error) {
	var requests []reconcile.Request
	var list = v1alpha1.ClusterLicenseList{}
	kind, err := k8s.GetKind(s, &v1alpha1.ElasticsearchCluster{})
	if err != nil {
		log.Error(err, "failed to get ElasticsearchCluster kind", "enterprise-license", license)
		return requests, err
	}

	// retries don't seem appropriate here as we are reading from a cache anyway
	err = c.List(&client.ListOptions{
		LabelSelector: NewClusterByLicenseSelector(license),
	}, &list)
	if err != nil {
		return requests, err
	}

	for _, cl := range list.Items {
		for _, o := range cl.GetOwnerReferences() {
			if o.Controller != nil && *o.Controller == true && o.Kind == kind {
				requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: cl.Namespace,
					Name:      o.Name,
				}})
			}
		}

	}
	return requests, nil

}
