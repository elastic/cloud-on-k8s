// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/apmserver"
)

// TestApmStandalone runs a test suite on an APM server that is not outputting to Elasticsearch
func TestApmStandalone(t *testing.T) {
	apmBuilder := apmserver.NewBuilder("standalone").
		WithElasticsearchRef(v1alpha1.ObjectSelector{}).
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		})

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
}

func TestApmStandaloneNoTLS(t *testing.T) {
	apmBuilder := apmserver.NewBuilder("standalone-no-tls").
		WithElasticsearchRef(v1alpha1.ObjectSelector{}).
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		}).
		WithHTTPCfg(v1alpha1.HTTPConfig{
			TLS: v1alpha1.TLSOptions{
				SelfSignedCertificate: &v1alpha1.SelfSignedCertificate{
					Disabled: true,
				},
			},
		})

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
}
