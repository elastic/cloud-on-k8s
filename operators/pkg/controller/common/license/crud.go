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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EnterpriseLicenseList(c k8s.Client) ([]SourceEnterpriseLicense, error) {
	licenseList := corev1.SecretList{}
	err := c.List(&client.ListOptions{
		LabelSelector: NewLicenseByTypeSelector(string(v1alpha1.LicenseTypeEnterprise)),
	}, &licenseList)
	if err != nil {
		return nil, err
	}
	var licenses []SourceEnterpriseLicense
	for _, ls := range licenseList.Items {
		parsed, err := ParseEnterpriseLicenses(ls.Data)
		if err != nil {
			return nil, pkgerrors.Wrapf(err, "unparseable license in %v", k8s.ExtractNamespacedName(&ls))
		}
		licenses = append(licenses, parsed...)
	}
	return licenses, nil
}

func TrialLicenses(c k8s.Client) ([]SourceEnterpriseLicense, error) {
	licenses, err := EnterpriseLicenseList(c)
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
