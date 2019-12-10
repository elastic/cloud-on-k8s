// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheme

import (
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
)

// SetupScheme sets up a scheme with all of the relevant types. This is only needed once for the manager but is often used for tests
// Afterwards you can use clientgoscheme.Scheme
func SetupScheme() error {
	err := clientgoscheme.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = apmv1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = commonv1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = esv1.AddToScheme(clientgoscheme.Scheme)
	if err != nil {
		return err
	}
	err = kbv1.AddToScheme(clientgoscheme.Scheme)
	return err
}
