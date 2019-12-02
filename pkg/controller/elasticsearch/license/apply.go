// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	commonlicense "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("elasticsearch-controller")

// isTrial returns true if an Elasticsearch license is of the trial type
func isTrial(l *esclient.License) bool {
	return l != nil && l.Type == string(commonlicense.ElasticsearchLicenseTypeTrial)
}

func applyLinkedLicense(
	c k8s.Client,
	esCluster types.NamespacedName,
	current *esclient.License,
	updater esclient.LicenseClient,
) error {
	var license corev1.Secret
	// the underlying assumption here is that either a user or a
	// license controller has created a cluster license in the
	// namespace of this cluster following the cluster-license naming
	// convention
	err := c.Get(
		types.NamespacedName{
			Namespace: esCluster.Namespace,
			Name:      v1beta1.LicenseSecretName(esCluster.Name),
		},
		&license,
	)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// no license linked to this cluster. Revert to basic.
			return startBasic(updater)
		}
		return err
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
	return updateLicense(esCluster, updater, current, desired)
}

func startBasic(updater esclient.LicenseClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()
	_, err := updater.StartBasic(ctx)
	if err != nil && esclient.IsForbidden(err) {
		// ES returns 403 + acknowledged: true (which we don't parse in case of error) if we are already in basic mode
		return nil
	}
	return pkgerrors.Wrap(err, "failed to revert to basic")
}

// updateLicense make the call to Elasticsearch to set the license. This function exists mainly to facilitate testing.
func updateLicense(
	esCluster types.NamespacedName,
	updater esclient.LicenseClient,
	current *esclient.License,
	desired esclient.License,
) error {
	if current != nil && (current.UID == desired.UID || (isTrial(current) && current.Type == desired.Type)) {
		return nil // we are done already applied
	}
	request := esclient.LicenseUpdateRequest{
		Licenses: []esclient.License{
			desired,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()

	if isTrial(&desired) {
		return pkgerrors.Wrap(startTrial(updater, esCluster), "failed to start trial")
	}

	response, err := updater.UpdateLicense(ctx, request)
	if err != nil {
		return pkgerrors.Wrap(err, fmt.Sprintf("failed to update license to %s", desired.Type))
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}

// startTrial starts the trial license after checking that the trial is not yet activated by directly hitting the
// Elasticsearch API.
func startTrial(c esclient.LicenseClient, esCluster types.NamespacedName) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()

	// Let's start the trial
	response, err := c.StartTrial(ctx)
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
