// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"errors"
	"sort"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type licenseWithTimeLeft struct {
	license    v1alpha1.ClusterLicenseSpec
	parentMeta metav1.ObjectMeta
	remaining  time.Duration
}

// BestMatch tries to find the best matching license given a list of enterprise licenses based on the
// desired license type and the remaining validity period of the license.
func BestMatch(
	licenses []v1alpha1.EnterpriseLicense,
) (v1alpha1.ClusterLicenseSpec, metav1.ObjectMeta, bool, error) {
	return bestMatchAt(time.Now(), licenses)
}

func bestMatchAt(
	now time.Time,
	licenses []v1alpha1.EnterpriseLicense,
) (v1alpha1.ClusterLicenseSpec, metav1.ObjectMeta, bool, error) {
	var license v1alpha1.ClusterLicenseSpec
	var parentMeta metav1.ObjectMeta
	if len(licenses) == 0 {
		// no license at all
		return license, parentMeta, false, nil
	}
	valid := filterValid(now, licenses)
	if len(valid) == 0 {
		return license, parentMeta, false, errors.New("no matching license found")
	}
	sort.Slice(valid, func(i, j int) bool {
		t1, t2 := v1alpha1.LicenseTypeOrder[valid[i].license.Type], v1alpha1.LicenseTypeOrder[valid[j].license.Type]
		if t1 != t2 { // sort by type
			return t1 < t2
		}
		// and by remaining time
		return valid[i].remaining < valid[j].remaining
	})
	best := valid[len(valid)-1]
	return best.license, best.parentMeta, true, nil
}

func filterValid(now time.Time, licenses []v1alpha1.EnterpriseLicense) []licenseWithTimeLeft {
	filtered := make([]licenseWithTimeLeft, 0)
	for _, el := range licenses {
		if el.IsValid(now) {
			for _, l := range el.Spec.ClusterLicenseSpecs {
				if l.IsValid(now) {
					filtered = append(filtered, licenseWithTimeLeft{
						license:    l,
						parentMeta: el.ObjectMeta,
						remaining:  l.ExpiryDate().Sub(now),
					})
				}
			}
		}
	}
	return filtered
}
