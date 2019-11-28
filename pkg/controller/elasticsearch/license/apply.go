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

func applyLinkedLicense(
	c k8s.Client,
	esCluster types.NamespacedName,
	current *esclient.License,
	updater esclient.LicenseUpdater,
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

	bytes, err := common_license.FetchLicenseData(license.Data)
	if err != nil {
		return err
	}

	var lic esclient.License
	err = json.Unmarshal(bytes, &lic)
	if err != nil {
		return pkgerrors.Wrap(err, "no valid license found in license secret")
	}
	return updateLicense(updater, current, lic)
}

func startBasic(updater esclient.LicenseUpdater) error {
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
	updater esclient.LicenseUpdater,
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
	response, err := updater.UpdateLicense(ctx, request)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}
