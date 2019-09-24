package scheme

import (
	apmv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	esv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	kbv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// SetupScheme sets up a scheme with all of the relevant types. This is only needed once for the manager but is often used for tests
// Afterwards you can use clientgoscheme.Scheme
func SetupScheme() error {
	err := clientgoscheme.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = apmv1alpha1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = commonv1alpha1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = esv1alpha1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = kbv1alpha1.AddToScheme(clientgoscheme.Scheme)
	return err
}
