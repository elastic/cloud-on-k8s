package license

import (
	"errors"
	"sort"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DesiredLicenseType *v1alpha1.LicenseType

func typeMatches(d DesiredLicenseType, t v1alpha1.LicenseType) bool {
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
		t1, t2 := v1alpha1.LicenseTypeOrder[valid[i].l.Spec.Type], v1alpha1.LicenseTypeOrder[valid[j].l.Spec.Type]
		if t1 != t2 { // sort by type
			return t1 < t2
		}
		// and by remaining time
		return valid[i].remaining < valid[j].remaining
	})
	return valid[len(valid)-1].l, nil
}

func filterValidForType(licenseType DesiredLicenseType, now time.Time, licenses []v1alpha1.EnterpriseLicense) []licenseWithTimeLeft {
	// assuming the typical enterprise license contains 3 sets of the 3 license types
	filtered := make([]licenseWithTimeLeft, 0, len(licenses)*3*3)
	for _, el := range licenses {
		if el.IsValid(now) {
			for _, l := range el.Spec.ClusterLicenseSpecs {
				if typeMatches(licenseType, l.Type) && l.IsValid(now, v1alpha1.NewSafetyMargin()) {
					filtered = append(filtered, licenseWithTimeLeft{
						l: v1alpha1.ClusterLicense{
							ObjectMeta: metav1.ObjectMeta{
								Name:      el.Name,
								Namespace: el.Namespace,
							},
							Spec: l,
						},
						remaining: l.ExpiryDate().Sub(now),
					})
				}
			}
		}
	}
	return filtered
}
