// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

// EnterpriseLicensesOrErrors lists all Enterprise licenses and all errors encountered during retrieval.
func EnterpriseLicensesOrErrors(c k8s.Client) ([]EnterpriseLicense, []error) {
	licenseList := corev1.SecretList{}
	matchingLabels := NewLicenseByScopeSelector(LicenseScopeOperator)
	err := c.List(&licenseList, matchingLabels)
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
	return licenses, utilerrors.NewAggregate(errors)
}

// TrialLicense returns the trial license, its containing secret or an error
func TrialLicense(c k8s.Client, nsn types.NamespacedName) (corev1.Secret, EnterpriseLicense, error) {
	var secret corev1.Secret
	err := c.Get(nsn, &secret)
	if err != nil {
		return corev1.Secret{}, EnterpriseLicense{}, err
	}
	if len(secret.Data) == 0 {
		// new trial license
		return secret, EnterpriseLicense{
			License: LicenseSpec{
				Type: LicenseTypeEnterpriseTrial,
			},
		}, nil
	}

	license, err := ParseEnterpriseLicense(secret.Data)
	if err != nil {
		return secret, EnterpriseLicense{}, err
	}
	if !license.IsTrial() {
		return secret, EnterpriseLicense{}, fmt.Errorf("%v is not a trial license", nsn)
	}
	return secret, license, nil
}

// CreateTrialLicense creates en empty secret with the correct meta data to start an enterprise trial
func CreateTrialLicense(c k8s.Client, namespace string) error {
	return c.Create(&corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      string(LicenseTypeEnterpriseTrial),
			Namespace: namespace,
			Labels: map[string]string{
				common.TypeLabelName: Type,
				LicenseLabelType:     string(LicenseTypeEnterpriseTrial),
			},
			Annotations: map[string]string{
				EULAAnnotation: EULAAcceptedValue,
			},
		},
	})
}

// UpdateEnterpriseLicense updates an Enterprise license wrapped in a secret.
func UpdateEnterpriseLicense(c k8s.Client, secret corev1.Secret, l EnterpriseLicense) error {
	bytes, err := json.Marshal(l)
	if err != nil {
		return pkgerrors.Wrap(err, "failed to marshal license")
	}
	secret.Data = map[string][]byte{
		FileName: bytes,
	}
	secret.Labels = maps.Merge(secret.Labels, LabelsForOperatorScope(l.License.Type))
	return c.Update(&secret)
}
