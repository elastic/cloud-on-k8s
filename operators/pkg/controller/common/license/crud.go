/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package license

import (
	"encoding/json"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	util_errors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnterpriseLicensesOrErrors lists all Enterprise licenses and all errors encountered during retrieval.
func EnterpriseLicensesOrErrors(c k8s.Client) ([]SourceEnterpriseLicense, []error) {
	licenseList := corev1.SecretList{}
	err := c.List(&client.ListOptions{
		LabelSelector: NewLicenseByTypeSelector(string(v1alpha1.LicenseTypeEnterprise)),
	}, &licenseList)
	if err != nil {
		return nil, []error{err}
	}
	var licenses []SourceEnterpriseLicense
	var errors []error
	for _, ls := range licenseList.Items {
		parsed, err := ParseEnterpriseLicenses(ls.Data)
		if err != nil {
			errors = append(errors, pkgerrors.Wrapf(err, "unparseable license in %v", k8s.ExtractNamespacedName(&ls)))
		} else {
			licenses = append(licenses, parsed...)
		}
	}
	return licenses, errors
}

// EnterpriseLicenses lists all Enterprise licenses or an aggregate error
func EnterpriseLicenses(c k8s.Client) ([]SourceEnterpriseLicense, error) {
	licenses, errors := EnterpriseLicensesOrErrors(c)
	return licenses, util_errors.NewAggregate(errors)
}

func TrialLicenses(c k8s.Client) ([]SourceEnterpriseLicense, error) {
	licenses, err := EnterpriseLicenses(c)
	if err != nil {
		return nil, err
	}
	var trials []SourceEnterpriseLicense
	for i, l := range licenses {
		if l.IsTrial() {
			trials = append(trials, licenses[i])
		}
	}
	return trials, nil
}

// CreateEnterpriseLicense creates an Enterprise license wrapped in a secret.
func CreateEnterpriseLicense(c k8s.Client, key types.NamespacedName, l SourceEnterpriseLicense) error {
	bytes, err := json.Marshal(l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to marshal license")
	}
	licenseSecret := corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Namespace: key.Namespace,
			Name:      key.Name,
			Labels:    LabelsForType(LicenseTypeEnterprise),
		},
		Data: map[string][]byte{
			key.Name: bytes,
		},
	}
	return c.Create(&licenseSecret)
}
