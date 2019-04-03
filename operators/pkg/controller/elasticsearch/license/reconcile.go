// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("license")

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	c k8s.Client,
	w watches.DynamicWatches,
	esCluster v1alpha1.Elasticsearch,
	clusterClient esclient.Client,
	current *esclient.License,
) error {
	if current == nil {
		// current license not discovered yet, no decision to take now
		return nil
	}
	switch esCluster.Spec.GetLicenseType() {
	case v1alpha1.LicenseTypeBasic:
		return reconcileBasicLicense(clusterClient, *current, esCluster)
	case v1alpha1.LicenseTypeTrial:
		return reconcileTrialLicense(clusterClient, *current, esCluster)
	case v1alpha1.LicenseTypeGold, v1alpha1.LicenseTypePlatinum:
		return reconcileGoldOrPlatinumLicense(c, w, esCluster, clusterClient, current)
	default:
		return nil
	}
}

func reconcileTrialLicense(
	clusterClient esclient.Client,
	current esclient.License,
	esCluster v1alpha1.Elasticsearch,
) error {
	if current.Type == v1alpha1.LicenseTypeTrial.String() {
		// nothing to do
		return nil
	}
	log.Info("Starting trial license", "cluster", esCluster.Name)
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return clusterClient.StartTrialLicense(ctx)
}

func reconcileBasicLicense(
	clusterClient esclient.Client,
	current esclient.License,
	esCluster v1alpha1.Elasticsearch,
) error {
	if current.Type == v1alpha1.LicenseTypeBasic.String() {
		// nothing to do
		return nil
	}
	log.Info("Starting basic license", "cluster", esCluster.Name)
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	return clusterClient.StartBasicLicense(ctx)
}

func reconcileGoldOrPlatinumLicense(
	c k8s.Client,
	w watches.DynamicWatches,
	esCluster v1alpha1.Elasticsearch,
	clusterClient esclient.Client,
	current *esclient.License,
) error {
	clusterName := k8s.ExtractNamespacedName(&esCluster)
	if err := ensureLicenseWatch(clusterName, w); err != nil {
		return err
	}
	return applyLinkedLicense(c, clusterName, func(license v1alpha1.ClusterLicense) error {
		sigResolver := secretRefResolver(c, clusterName.Namespace, license.Spec.SignatureRef)
		return updateLicense(clusterClient, current, license, sigResolver)
	})
}
