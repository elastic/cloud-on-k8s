// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"encoding/json"
	"fmt"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	commonlicense "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

// isTrial returns true if an Elasticsearch license is of the trial type
func isTrial(l esclient.License) bool {
	return l.Type == string(esclient.ElasticsearchLicenseTypeTrial)
}

// isECKManagedTrial returns true if this is a trial started via the internal trial mechanism in the operator. We use an
// empty license with only the type field populated to indicate to the Elasticsearch controller that a self-generated
// trial in Elasticsearch should be started. This can be done only once.
func isECKManagedTrial(l esclient.License) bool {
	return isTrial(l) && l.Signature == "" && l.UID == "" && l.ExpiryDateInMillis == 0 && l.StartDateInMillis == 0
}

// isBasic returns true if an Elasticsearch license is of the basic type
func isBasic(l esclient.License) bool {
	return l.Type == string(esclient.ElasticsearchLicenseTypeBasic)
}

func applyLinkedLicense(
	ctx context.Context,
	c k8s.Client,
	esCluster types.NamespacedName,
	updater esclient.LicenseClient,
	currentLicense esclient.License,
) error {
	// get the expected license
	// the underlying assumption here is that either a user or a
	// license controller has created a cluster license in the
	// namespace of this cluster following the cluster-license naming
	// convention
	var license corev1.Secret
	err := c.Get(ctx,
		types.NamespacedName{
			Namespace: esCluster.Namespace,
			Name:      esv1.LicenseSecretName(esCluster.Name),
		},
		&license,
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err != nil && apierrors.IsNotFound(err) {
		// no license expected, let's look at the current cluster license
		switch {
		case isBasic(currentLicense):
			// nothing to do
			return nil
		case isTrial(currentLicense):
			// Elasticsearch reports a trial license, but there's no ECK enterprise trial requested.
			// This can be the case if:
			// - an ECK trial was started previously, then stopped (secret removed)
			// - the user manually started a trial at the stack level (eg. by clicking a button in Kibana when
			// trying to access a commercial feature). While this is not a supported use case,
			// we tolerate it to avoid a bad user experience because trials can only be started once.
			ulog.FromContext(ctx).V(1).Info("Preserving existing stack-level trial license",
				"namespace", esCluster.Namespace, "es_name", esCluster.Name)
			return nil
		default:
			// revert the current license to basic
			return startBasic(ctx, updater)
		}
	}

	bytes, err := commonlicense.FetchLicenseData(license.Data)
	if err != nil {
		return err
	}

	var desired esclient.License
	err = json.Unmarshal(bytes, &desired)
	if err != nil {
		return pkgerrors.Wrap(err, "no valid license found in license secret")
	}
	return updateLicense(ctx, esCluster, updater, currentLicense, desired)
}

func startBasic(ctx context.Context, updater esclient.LicenseClient) error {
	_, err := updater.StartBasic(ctx)
	if err != nil && esclient.IsForbidden(err) {
		// ES returns 403 + acknowledged: true (which we don't parse in case of error) if we are already in basic mode
		return nil
	}
	return pkgerrors.Wrap(err, "failed to revert to basic")
}

// updateLicense make the call to Elasticsearch to set the license. This function exists mainly to facilitate testing.
func updateLicense(
	ctx context.Context,
	esCluster types.NamespacedName,
	updater esclient.LicenseClient,
	current esclient.License,
	desired esclient.License,
) error {
	if current.UID == desired.UID || (isTrial(current) && current.Type == desired.Type) {
		return nil // we are done already applied
	}
	request := esclient.LicenseUpdateRequest{
		Licenses: []esclient.License{
			desired,
		},
	}

	if isECKManagedTrial(desired) {
		// start a self-generated trial in Elasticsearch, this can only be done once.
		return pkgerrors.Wrap(startTrial(ctx, updater, esCluster), "failed to start trial")
	}

	response, err := updater.UpdateLicense(ctx, request)
	if err != nil {
		return pkgerrors.Wrap(err, fmt.Sprintf("failed to update license to %s", desired.Type))
	}
	if !response.IsSuccess() {
		return pkgerrors.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}

// startTrial starts the trial license after checking that the trial is not yet activated by directly hitting the
// Elasticsearch API.
func startTrial(ctx context.Context, c esclient.LicenseClient, esCluster types.NamespacedName) error {
	response, err := c.StartTrial(ctx)
	log := ulog.FromContext(ctx)
	if err != nil && esclient.IsForbidden(err) {
		log.Info("failed to start trial most likely because trial was activated previously",
			"err", err.Error(),
			"namespace", esCluster.Namespace,
			"name", esCluster.Name,
		)
		return nil
	}
	if response.IsSuccess() {
		log.Info(
			"Elasticsearch trial license activated",
			"namespace", esCluster.Namespace,
			"name", esCluster.Name,
		)
	}
	return err
}
