// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	common_license "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// isTrial returns true if an Elasticsearch license is of the trial type
func isTrial(l *esclient.License) bool {
	return l != nil && l.Type == string(common_license.LicenseTypeEnterpriseTrial)
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

	// Shortcut if a trial license has already been started
	if _, ok := license.Annotations[common_license.TrialLicenseStartedAnnotation]; ok {
		log.V(1).Info("Trial license already started")
		return nil
	}

	bytes, err := common_license.FetchLicenseData(license.Data)
	if err != nil {
		return err
	}

	var lic esclient.License
	err = json.Unmarshal(bytes, &lic)
	if err != nil {
		return pkgerrors.Wrap(err, "no valid license found in license secret")
	}

	err = updater(lic)
	if err != nil {
		return err
	}

	// Store that the trial license has been started in an annotation
	if isTrial(&lic) {
		if license.Annotations == nil {
			license.Annotations = map[string]string{}
		}
		license.Annotations[common_license.TrialLicenseStartedAnnotation] = "true"

		err = c.Update(&license)
		if err != nil {
			log.Info("Error when updating the license secret", "err", err.Error())
			return err
		}
	}

	return nil
}

// updateLicense make the call to Elasticsearch to set the license. This function exists mainly to facilitate testing.
func updateLicense(
	c esclient.Client,
	current *esclient.License,
	desired esclient.License,
) error {
	if current != nil && current.UID == desired.UID {
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
		response, err := c.StartTrial(ctx)
		if err != nil {
			return err
		}
		if !response.IsSuccess() {
			return fmt.Errorf("failed to start trial license: %s", response.ErrorMessage)
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
