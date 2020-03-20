// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheme

import (
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
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
	if err != nil {
		return err
	}
	err = entsv1beta1.AddToScheme(clientgoscheme.Scheme)
	return err
}

// SetupV1beta1Scheme sets up a scheme with v1beta1 resources.
// Should only be used for the v1beta1 admission webhook.
// The operator should otherwise deal with a single resource version.
func SetupV1beta1Scheme() error {
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
	if err != nil {
		return err
	}
	err = entsv1beta1.AddToScheme(clientgoscheme.Scheme)
	return err
}
