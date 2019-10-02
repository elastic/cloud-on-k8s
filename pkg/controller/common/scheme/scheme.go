// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheme

import (
	apmv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// SetupScheme sets up a scheme with all of the relevant types. This is only needed once for the manager but is often used for tests
// Afterwards you can use clientgoscheme.Scheme
func SetupScheme() error {
	err := clientgoscheme.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = apmv1beta1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = commonv1beta1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = esv1beta1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = kbv1beta1.AddToScheme(clientgoscheme.Scheme)
	return err
}
