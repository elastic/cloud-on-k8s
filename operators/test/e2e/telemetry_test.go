// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"errors"
	"strings"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/stack"
)

func TestTelemetry(t *testing.T) {
	k8s := helpers.NewK8sClientOrFatal()

	s := stack.NewStackBuilder("test-telemetry").
		WithESMasterDataNodes(1, stack.DefaultResources).
		WithKibana(1)

	helpers.TestStepList{}.
		WithSteps(stack.InitTestSteps(s, k8s)...).
		WithSteps(stack.CreationTestSteps(s, k8s)...).
		WithSteps(
			helpers.TestStep{
				Name: "Kibana should expose eck info in telemetry data",
				Test: helpers.Eventually(func() error {

					uri := "/api/telemetry/v1/clusters/_stats"
					payload := `{"timeRange":{"min":"0","max":"0"}}`
					body, err := stack.KibanaDoReq(s, "POST", uri, []byte(payload))
					if err != nil {
						return err
					}

					// hack-ish test but simple
					json := `"static_telemetry":{"eck":{"distribution"`
					if !strings.Contains(string(body), json) {
						return errors.New("eck info not found in telemetry data")
					}

					return nil
				}),
			},
		).
		RunSequential(t)
}
