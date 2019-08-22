// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"context"
	"encoding/json"
	"fmt"

	common_license "github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

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
			Name:      name.LicenseSecretName(esCluster.Name),
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
	if len(license.Data) != 1 {
		return fmt.Errorf(
			"linked Elasticsearch license secret needs to contain exactly one file called %s",
			common_license.FileName,
		)
	}
	bytes := license.Data[common_license.FileName]
	var lic esclient.License
	err = json.Unmarshal(bytes, &lic)
	if err != nil {
		return pkgerrors.Wrap(err, "no valid license found in license secret")
	}
	return updater(lic)
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
	response, err := c.UpdateLicense(ctx, request)
	if err != nil {
		return err
	}
	if !response.IsSuccess() {
		return fmt.Errorf("failed to apply license: %s", response.LicenseStatus)
	}
	return nil
}
