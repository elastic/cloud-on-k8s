// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"

	"github.com/pkg/errors"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// Reconcile reconciles the current Elasticsearch license with the desired one.
func Reconcile(
	ctx context.Context,
	c k8s.Client,
	esCluster esv1.Elasticsearch,
	clusterClient esclient.Client,
	currentLicense esclient.License,
) error {
	clusterName := k8s.ExtractNamespacedName(&esCluster)
	return applyLinkedLicense(ctx, c, clusterName, clusterClient, currentLicense)
}

// CheckElasticsearchLicense checks that Elasticsearch is licensed, which ensures that the operator is communicating
// with a supported Elasticsearch distribution and that Elasticsearch is reachable.
func CheckElasticsearchLicense(ctx context.Context, clusterClient esclient.LicenseClient) (esclient.License, error) {
	esReachable := true
	supportedDistribution := true
	currentLicense, err := clusterClient.GetLicense(ctx)
	if err != nil {
		switch {
		case esclient.IsUnauthorized(err):
			err = errors.New("unauthorized access, unable to verify Elasticsearch license, check your security configuration")
		case esclient.IsForbidden(err):
			err = errors.New("forbidden access, unable to verify Elasticsearch license, check your security configuration")
		case esclient.IsNotFound(err):
			// 404 may happen if the master node is generating a new cluster state
		case esclient.Is4xx(err):
			supportedDistribution = false
			err = errors.Wrap(err, "unable to verify Elasticsearch license")
		default:
			esReachable = false
		}
		return esclient.License{}, &GetLicenseError{
			msg:                   err.Error(),
			SupportedDistribution: supportedDistribution,
			EsReachable:           esReachable,
		}
	}
	return currentLicense, nil
}

type GetLicenseError struct {
	msg                   string
	SupportedDistribution bool
	EsReachable           bool
}

func (e *GetLicenseError) Error() string {
	return e.msg
}
