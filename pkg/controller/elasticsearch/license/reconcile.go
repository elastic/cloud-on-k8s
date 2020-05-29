// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	ctx context.Context,
	c k8s.Client,
	esCluster esv1.Elasticsearch,
	clusterClient esclient.Client,
) error {
	clusterName := k8s.ExtractNamespacedName(&esCluster)
	return applyLinkedLicense(ctx, c, clusterName, clusterClient)
}
