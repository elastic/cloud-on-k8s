// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"sort"
	"time"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

type licenseWithTimeLeft struct {
	license   client.License
	parentUID string
	remaining time.Duration
}

// BestMatch tries to find the best matching license given a list of enterprise licenses based on the
// the minimal Elasticsearch version in the cluster, the desired license type and the remaining validity
// period of the license.
// Returns the license, parent license UID, a bool indicating a match was found and an optional error.
func BestMatch(
	ctx context.Context,
	minVersion *version.Version,
	licenses []EnterpriseLicense,
	filter func(EnterpriseLicense) (bool, error),
) (client.License, string, bool) {
	return bestMatchAt(ctx, time.Now(), minVersion, licenses, filter)
}

func bestMatchAt(
	ctx context.Context,
	now time.Time,
	minVersion *version.Version,
	licenses []EnterpriseLicense,
	filter func(EnterpriseLicense) (bool, error),
) (client.License, string, bool) {
	var license client.License
	var parentMeta string
	if len(licenses) == 0 {
		// no license at all
		return license, parentMeta, false
	}
	valid := filterValid(ctx, now, minVersion, licenses, filter)
	if len(valid) == 0 {
		ulog.FromContext(ctx).Info("No matching license found", "num_licenses", len(licenses))
		return license, parentMeta, false
	}
	sort.Slice(valid, func(i, j int) bool {
		t1, t2 := client.ElasticsearchLicenseTypeOrder[client.ElasticsearchLicenseType(valid[i].license.Type)],
			client.ElasticsearchLicenseTypeOrder[client.ElasticsearchLicenseType(valid[j].license.Type)]
		if t1 != t2 { // sort by type
			return t1 < t2
		}
		// and by remaining time
		return valid[i].remaining < valid[j].remaining
	})
	best := valid[len(valid)-1]
	return best.license, best.parentUID, true
}

func filterValid(ctx context.Context, now time.Time, minVersion *version.Version, licenses []EnterpriseLicense, filter func(EnterpriseLicense) (bool, error)) []licenseWithTimeLeft {
	log := ulog.FromContext(ctx)
	filtered := make([]licenseWithTimeLeft, 0)
	for _, el := range licenses {
		if !el.IsValid(now) {
			log.V(1).Info("Discarding invalid Enterprise license (validity period)", "eck_license", el.License.UID)
			continue
		}

		ok, err := filter(el)
		if err != nil {
			log.Error(err, "while checking license validity")
			continue
		}
		if !ok {
			log.V(1).Info("Discarding invalid Enterprise license (signature)", "eck_license", el.License.UID)
			continue
		}

		// Shortcut if it's an ECK managed trial license
		if el.IsECKManagedTrial() {
			filtered = append(filtered, licenseWithTimeLeft{
				// For a trial, only the type is used, the license will be generated by ES
				license: client.License{
					Type: string(client.ElasticsearchLicenseTypeTrial),
				},
				parentUID: el.License.UID,
				remaining: el.ExpiryTime().Sub(now),
			})
			continue
		}

		for _, l := range el.License.ClusterLicenses {
			if l.License.IsValid(now) && l.License.IsSupported(minVersion) {
				filtered = append(filtered, licenseWithTimeLeft{
					license:   l.License,
					parentUID: el.License.UID,
					remaining: l.License.ExpiryTime().Sub(now),
				})
			}
		}
	}
	return filtered
}
