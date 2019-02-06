// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.
package license

import (
	"context"

	"k8s.io/client-go/util/retry"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func listAffectedLicenses(c client.Client, license types.NamespacedName) []v1alpha1.ClusterLicense {
	var list = v1alpha1.ClusterLicenseList{}
	// errors here are unlikely to be recoverable try again anyway
	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (done bool, err error) {
		err = c.List(context.Background(), &client.ListOptions{
			LabelSelector: NewClusterByLicenseSelector(license),
		}, &list)
		return err == nil, err

	})
	if err != nil {
		log.Error(err, "failed to list affected clusters", "enterprise-license", license)
	}
	return list.Items

}
