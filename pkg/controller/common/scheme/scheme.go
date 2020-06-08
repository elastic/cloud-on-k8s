// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package scheme

import (
	"sync"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var addToScheme sync.Once
var addToSchemeV1beta1 sync.Once

// SetupScheme sets up a scheme with all of the relevant types. This is only needed once for the manager but is often used for tests
// Afterwards you can use clientgoscheme.Scheme
func SetupScheme() {
	addToScheme.Do(func() {
		err := clientgoscheme.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			// this should never happen
			panic(err)
		}

		err = apmv1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = commonv1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = esv1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = kbv1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = entv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = beatv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
	})
}

// SetupV1beta1Scheme sets up a scheme with v1beta1 resources.
// Should only be used for the v1beta1 admission webhook.
// The operator should otherwise deal with a single resource version.
func SetupV1beta1Scheme() {
	addToSchemeV1beta1.Do(func() {
		err := clientgoscheme.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			// this should never happen
			panic(err)
		}
		err = apmv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = commonv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = esv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = kbv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = entv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
		err = beatv1beta1.AddToScheme(clientgoscheme.Scheme)
		if err != nil {
			panic(err)
		}
	})
}
