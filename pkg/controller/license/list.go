// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func listAffectedLicenses(c k8s.Client, licenseName string) ([]reconcile.Request, error) {
	var list = corev1.SecretList{}
	// list all cluster licenses referencing the given enterprise license
	err := c.List(&client.ListOptions{
		LabelSelector: license.NewLicenseByNameSelector(licenseName),
	}, &list)
	if err != nil {
		return nil, err
	}

	// enqueue a reconcile request for each cluster license found leveraging the fact that cluster and license
	// share the same name
	requests := make([]reconcile.Request, len(list.Items))
	for i, cl := range list.Items {
		requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: cl.Namespace,
			Name:      cl.Name,
		}}
	}
	return requests, nil
}
