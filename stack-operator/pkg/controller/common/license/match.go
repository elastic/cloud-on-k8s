package license

import (
	"errors"
	"sort"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
)

type DesiredLicenseType *string

func typeMatches(d DesiredLicenseType, t string) bool {
	return d == nil || *d == t
}

type licenseWithTimeLeft struct {
	l         v1alpha1.ClusterLicense
	remaining time.Duration
}

func BestMatch(
	licenses []v1alpha1.EnterpriseLicense,
	desiredLicense DesiredLicenseType,
) (v1alpha1.ClusterLicense, error) {
	return bestMatchAt(time.Now(), licenses, desiredLicense)
}

func bestMatchAt(
	now time.Time,
	licenses []v1alpha1.EnterpriseLicense,
	desiredLicense DesiredLicenseType,
) (v1alpha1.ClusterLicense, error) {
	var license v1alpha1.ClusterLicense
	valid := filterValidForType(desiredLicense, now, licenses)
	if len(valid) == 0 {
		return license, errors.New("no matching license found")
	}
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].remaining < valid[j].remaining
	})
	return valid[len(valid)-1].l, nil
}

func filterValidForType(licenseType DesiredLicenseType, now time.Time, licenses []v1alpha1.EnterpriseLicense) []licenseWithTimeLeft {
	// assuming the typical enterprise license contains 3 sets of the 3 license types
	filtered := make([]licenseWithTimeLeft, len(licenses)*3*3)
	for _, el := range licenses {
		if el.Valid(now) {
			for _, l := range el.Spec.ClusterLicenses {
				if typeMatches(licenseType, l.Spec.Type) && l.IsValidAt(now, v1alpha1.NewSafetyMargin()) {
					filtered = append(filtered, licenseWithTimeLeft{
						l:         l,
						remaining: time.Unix(0, l.Spec.ExpiryDateInMillis*int64(time.Millisecond)).Sub(now),
					})
				}
			}
		}
	}
	return filtered
}
