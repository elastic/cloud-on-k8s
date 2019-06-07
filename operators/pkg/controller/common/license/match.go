// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"errors"
	"sort"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
)

type licenseWithTimeLeft struct {
	license   client.License
	parentUID string
	remaining time.Duration
}

// BestMatch tries to find the best matching license given a list of enterprise licenses based on the
// desired license type and the remaining validity period of the license.
func BestMatch(
	licenses []EnterpriseLicense,
	filter func(EnterpriseLicense) (bool, error),
) (client.License, string, bool, error) {
	return bestMatchAt(time.Now(), licenses, filter)
}

func bestMatchAt(
	now time.Time,
	licenses []EnterpriseLicense,
	filter func(EnterpriseLicense) (bool, error),
) (client.License, string, bool, error) {
	var license client.License
	var parentMeta string
	if len(licenses) == 0 {
		// no license at all
		return license, parentMeta, false, nil
	}
	valid := filterValid(now, licenses, filter)
	if len(valid) == 0 {
		return license, parentMeta, false, errors.New("no matching license found")
	}
	sort.Slice(valid, func(i, j int) bool {
		t1, t2 := v1alpha1.LicenseTypeOrder[v1alpha1.LicenseType(valid[i].license.Type)],
			v1alpha1.LicenseTypeOrder[v1alpha1.LicenseType(valid[j].license.Type)]
		if t1 != t2 { // sort by type
			return t1 < t2
		}
		// and by remaining time
		return valid[i].remaining < valid[j].remaining
	})
	best := valid[len(valid)-1]
	return best.license, best.parentUID, true, nil
}

func filterValid(now time.Time, licenses []EnterpriseLicense, filter func(EnterpriseLicense) (bool, error)) []licenseWithTimeLeft {
	filtered := make([]licenseWithTimeLeft, 0)
	for _, el := range licenses {
		if el.IsValid(now) {
			ok, err := filter(el)
			if err != nil {
				log.Error(err, "while checking license validity")
				continue
			}
			if !ok {
				continue
			}
			for _, l := range el.License.ClusterLicenses {
				if l.License.IsValid(now) {
					filtered = append(filtered, licenseWithTimeLeft{
						license:   l.License,
						parentUID: el.License.UID,
						remaining: l.License.ExpiryTime().Sub(now),
					})
				}
			}
		}
	}
	return filtered
}
