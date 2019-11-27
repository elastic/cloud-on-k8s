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

var log = logf.Log.WithName("elasticsearch-license")

// isTrial returns true if an Elasticsearch license is of the trial type
func isTrial(l *esclient.License) bool {
	return l != nil && l.Type == string(commonlicense.ElasticsearchLicenseTypeTrial)
}

func applyLinkedLicense(
	c k8s.Client,
	esCluster types.NamespacedName,
	updater func(license esclient.License) error,
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
			// no license linked to this cluster. Expected for clusters running on basic or trial.
			return nil
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

	err = updater(desired)
	if err != nil {
		return err
	}

	return nil
}

// updateLicense make the call to Elasticsearch to set the license. This function exists mainly to facilitate testing.
func updateLicense(
	c esclient.Client,
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
		err := startTrial(c)
		if err != nil {
			return err
		}
		return nil
	}

	response, err := c.UpdateLicense(ctx, request)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}

// startTrial starts the trial license after checking that the trial is not yet activated by directly hitting the
// Elasticsearch API.
func startTrial(c esclient.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), esclient.DefaultReqTimeout)
	defer cancel()

	// Check the current license
	license, err := c.GetLicense(ctx)
	if err != nil {
		return err
	}
	if isTrial(&license) {
		// Trial already activated
		return nil
	}

	// Let's start the trial
	response, err := c.StartTrial(ctx)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to start trial license: %s", response.ErrorMessage)
	}

	log.Info("Trial license started")
	return nil
}
