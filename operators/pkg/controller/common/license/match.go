package license

import (
	"errors"
	"sort"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func typeMatches(d v1alpha1.LicenseType, t v1alpha1.LicenseType) bool {
	// desired type is either any type expressed by the default string "" or equal to the type of the license
	return d == "" || v1alpha1.LicenseType(d) == t
}

type licenseWithTimeLeft struct {
	license    v1alpha1.ClusterLicenseSpec
	parentMeta metav1.ObjectMeta
	remaining  time.Duration
}

// BestMatch tries to find the best matching license given a list of enterprise licenses based on the
// desired license type and the remaining validity period of the license.
func BestMatch(
	licenses []v1alpha1.EnterpriseLicense,
	desiredLicense v1alpha1.LicenseType,
) (v1alpha1.ClusterLicenseSpec, metav1.ObjectMeta, error) {
	return bestMatchAt(time.Now(), licenses, desiredLicense)
}

func bestMatchAt(
	now time.Time,
	licenses []v1alpha1.EnterpriseLicense,
	desiredLicense v1alpha1.LicenseType,
) (v1alpha1.ClusterLicenseSpec, metav1.ObjectMeta, error) {
	var license v1alpha1.ClusterLicenseSpec
	var parentMeta metav1.ObjectMeta
	valid := filterValidForType(desiredLicense, now, licenses)
	if len(valid) == 0 {
		return license, parentMeta, errors.New("no matching license found")
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
	return best.license, best.parentMeta, nil
}

func filterValidForType(desiredLicense v1alpha1.LicenseType, now time.Time, licenses []v1alpha1.EnterpriseLicense) []licenseWithTimeLeft {
	filtered := make([]licenseWithTimeLeft, 0)
	for _, el := range licenses {
		if el.IsValid(now) {
			for _, l := range el.Spec.ClusterLicenseSpecs {
				if typeMatches(desiredLicense, l.Type) && l.IsValid(now) {
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
