/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	util_errors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnterpriseLicensesOrErrors lists all Enterprise licenses and all errors encountered during retrieval.
func EnterpriseLicensesOrErrors(c k8s.Client) ([]EnterpriseLicense, []error) {
	licenseList := corev1.SecretList{}
	err := c.List(&client.ListOptions{
		LabelSelector: NewLicenseByTypeSelector(string(LicenseTypeEnterprise)),
	}, &licenseList)
	if err != nil {
		return nil, []error{err}
	}
	var licenses []EnterpriseLicense
	var errors []error
	for _, ls := range licenseList.Items {
		parsed, err := ParseEnterpriseLicense(ls.Data)
		if err != nil {
			errors = append(errors, pkgerrors.Wrapf(err, "unparseable license in %v", k8s.ExtractNamespacedName(&ls)))
		} else {
			licenses = append(licenses, parsed)
		}
	}
	return licenses, errors
}

// EnterpriseLicenses lists all Enterprise licenses or an aggregate error
func EnterpriseLicenses(c k8s.Client) ([]EnterpriseLicense, error) {
	licenses, errors := EnterpriseLicensesOrErrors(c)
	return licenses, util_errors.NewAggregate(errors)
}

func TrialLicense(c k8s.Client, nsn types.NamespacedName) (EnterpriseLicense, error) {
	var secret corev1.Secret
	err := c.Get(nsn, &secret)
	if err != nil {
		return EnterpriseLicense{}, err
	}
	if len(secret.Data) == 0 {
		// new trial license
		return EnterpriseLicense{
			License: LicenseSpec{
				Type: LicenseTypeEnterpriseTrial,
			},
		}, nil
	}

	license, err := ParseEnterpriseLicense(secret.Data)
	if err != nil {
		return EnterpriseLicense{}, err
	}
	if !license.IsTrial() {
		return EnterpriseLicense{}, fmt.Errorf("%v is not a trial license", nsn)
	}
	return license, nil
}

// CreateTrialLicense create en empty secret with the correct meta data to start an enterprise trial
func CreateTrialLicense(c k8s.Client, namespace string) error {
	return c.Create(&corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      string(LicenseTypeEnterpriseTrial),
			Namespace: namespace,
			Labels: map[string]string{
				common.TypeLabelName: Type,
			},
			Annotations: map[string]string{
				"elastic.co/eula": "accepted",
			},
		},
	})
}

// CreateEnterpriseLicense creates an Enterprise license wrapped in a secret.
func CreateEnterpriseLicense(c k8s.Client, key types.NamespacedName, l EnterpriseLicense) error {
	bytes, err := json.Marshal(l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to marshal license")
	}
	return c.Create(&corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: key.Namespace,
			Name:      key.Name,
			Labels:    LabelsForType(LicenseLabelEnterprise),
		},
		Data: map[string][]byte{
			LicenseFileName: bytes,
		},
	})
}

// UpdateEnterpriseLicense creates an Enterprise license wrapped in a secret.
func UpdateEnterpriseLicense(c k8s.Client, key types.NamespacedName, l EnterpriseLicense) error {
	bytes, err := json.Marshal(l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to marshal license")
	}
	var secret corev1.Secret
	err = c.Get(key, &secret)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to fetch license secret")
	}
	secret.Data = map[string][]byte{
		LicenseFileName: bytes,
	}
	secret.Labels = LabelsForType(LicenseLabelEnterprise)
	return c.Update(&secret)
}
