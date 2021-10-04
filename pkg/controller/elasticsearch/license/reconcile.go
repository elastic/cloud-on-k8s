// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	ctx context.Context,
	c k8s.Client,
	esCluster esv1.Elasticsearch,
	clusterClient esclient.Client,
) (bool, error) {
	currentLicense, supportedDistribution, err := checkElasticsearchLicense(ctx, clusterClient)
	if err != nil {
		return supportedDistribution, err
	}

	clusterName := k8s.ExtractNamespacedName(&esCluster)
	return true, applyLinkedLicense(ctx, c, clusterName, clusterClient, currentLicense)
}

// checkElasticsearchLicense checks that Elasticsearch is licensed, which ensures that the operator is communicating
// with a supported Elasticsearch distribution
func checkElasticsearchLicense(ctx context.Context, clusterClient esclient.LicenseClient) (esclient.License, bool, error) {
	supportedDistribution := true
	currentLicense, err := clusterClient.GetLicense(ctx)
	if err != nil {
		if esclient.IsUnauthorized(err) {
			err = errors.New("unauthorized access, unable to verify Elasticsearch license, check your security configuration")
		} else if esclient.IsForbidden(err) {
			err = errors.New("forbidden access, unable to verify Elasticsearch license, check your security configuration")
		} else if esclient.IsNotFound(err) {
			// 404 may happen if the master node is generating a new cluster state
		} else if esclient.Is4xx(err) {
			supportedDistribution = false
			err = errors.Wrap(err, "unable to verify Elasticsearch license")
		}
	}
	return currentLicense, supportedDistribution, err
}
