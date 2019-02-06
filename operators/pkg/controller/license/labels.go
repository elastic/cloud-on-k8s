package license

import (
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

const EnterpriseLicenseLabelName = "k8s.elastic.co/enterprise-license-name"

func NewClusterByLicenseSelector(license types.NamespacedName) labels.Selector {
	return labels.Set(map[string]string{EnterpriseLicenseLabelName: license.Name}).AsSelector()
}
