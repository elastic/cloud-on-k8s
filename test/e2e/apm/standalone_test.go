// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/apmserver"
)

// TestApmStandalone runs a test suite on an APM server that is not outputting to Elasticsearch
func TestAPMStandalone(t *testing.T) {
	apmBuilder := apmserver.NewBuilder("standalone").
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		}).
		WithoutIntegrationCheck()

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
}

func TestAPMStandaloneWithRUM(t *testing.T) {
	apmBuilder := apmserver.NewBuilder("standalone-with-rum").
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		}).
		WithRUM(true).
		WithoutIntegrationCheck()

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
}

func TestAPMStandaloneNoTLS(t *testing.T) {
	apmBuilder := apmserver.NewBuilder("standalone-no-tls").
		WithConfig(map[string]interface{}{
			"output.console": map[string]interface{}{
				"pretty": true,
			},
		}).
		WithHTTPCfg(commonv1.HTTPConfig{
			TLS: commonv1.TLSOptions{
				SelfSignedCertificate: &commonv1.SelfSignedCertificate{
					Disabled: true,
				},
			},
		}).
		WithoutIntegrationCheck()

	test.Sequence(nil, test.EmptySteps, apmBuilder).
		RunSequential(t)
}
