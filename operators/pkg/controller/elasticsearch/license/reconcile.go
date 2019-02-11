// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
)

const (
	// Expectation the license type expected to be attached to the cluster.
	Expectation = "k8s.elastic.co/expected-license"
	// Trial is a 30-day trial license type.
	Trial = "trial"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	c k8s.Client,
	w watches.DynamicWatches,
	esCluster v1alpha1.ElasticsearchCluster,
	clusterClient *esclient.Client,
	current *esclient.License,
) error {
	// This is mostly for dev mode, no license management when trial is requested
	if esCluster.Labels[Expectation] == Trial {
		return nil
	}

	clusterName := k8s.ExtractNamespacedName(&esCluster)
	if err := ensureLicenseWatch(clusterName, w); err != nil {
		return err
	}
	return applyLinkedLicense(c, clusterName, func(license v1alpha1.ClusterLicense) error {
		sigResolver := secretRefResolver(c, clusterName.Namespace, license.Spec.SignatureRef)
		return updateLicense(clusterClient, current, license, sigResolver)
	})
}
