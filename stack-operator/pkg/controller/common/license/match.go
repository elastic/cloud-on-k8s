package license

import "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"

type DesiredLicenseType *string

func BestMatch(licenses []v1alpha1.EnterpriseLicense, desiredLicense DesiredLicenseType) *v1alpha1.ClusterLicense {
	panic("implement me")
}

