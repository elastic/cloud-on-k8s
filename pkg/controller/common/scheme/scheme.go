// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package scheme

import (
	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	apmv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1beta1"
	easv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/autoscaling/v1alpha1"
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1beta1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	esv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1beta1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	entv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1beta1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	kbv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1beta1"
	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
)

var addToScheme sync.Once
var addToSchemeV1beta1 sync.Once

func mustAddSchemeOnce(once *sync.Once, schemes []func(scheme *runtime.Scheme) error) {
	once.Do(func() {
		for _, s := range schemes {
			if err := s(clientgoscheme.Scheme); err != nil {
				panic(err)
			}
		}
	})
}

// SetupScheme sets up a scheme with all of the relevant types. This is only needed once for the manager but is often used for tests
// Afterwards you can use clientgoscheme.Scheme
func SetupScheme() {
	schemes := []func(scheme *runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apmv1.AddToScheme,
		commonv1.AddToScheme,
		esv1.AddToScheme,
		easv1alpha1.AddToScheme,
		kbv1.AddToScheme,
		entv1.AddToScheme,
		beatv1beta1.AddToScheme,
		agentv1alpha1.AddToScheme,
		emsv1alpha1.AddToScheme,
		policyv1alpha1.AddToScheme,
		logstashv1alpha1.AddToScheme,
	}
	mustAddSchemeOnce(&addToScheme, schemes)
}

// SetupV1beta1Scheme sets up a scheme with v1beta1 resources.
// Should only be used for the v1beta1 admission webhook.
// The operator should otherwise deal with a single resource version.
func SetupV1beta1Scheme() {
	schemes := []func(scheme *runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apmv1beta1.AddToScheme,
		commonv1beta1.AddToScheme,
		esv1beta1.AddToScheme,
		kbv1beta1.AddToScheme,
		entv1beta1.AddToScheme,
		beatv1beta1.AddToScheme,
		agentv1alpha1.AddToScheme,
		logstashv1alpha1.AddToScheme,
	}
	mustAddSchemeOnce(&addToSchemeV1beta1, schemes)
}
