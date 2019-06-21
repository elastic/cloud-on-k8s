// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/about"
	es "github.com/elastic/cloud-on-k8s/operators/test/e2e/elasticsearch"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/helpers"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
	kb "github.com/elastic/cloud-on-k8s/operators/test/e2e/kibana"
)

func TestTelemetry(t *testing.T) {
	k := helpers.NewK8sClientOrFatal()

	b1 := es.NewBuilder("test-telemetry").
		WithESMasterDataNodes(1, es.DefaultResources)
	b2 := kb.NewBuilder("test-telemetry").
		WithKibana(1)

	helpers.TestStepList{}.
		WithSteps(es.InitTestSteps(b1, k)...).
		WithSteps(kb.InitTestSteps(b2, k)...).
		WithSteps(es.CreationTestSteps(b1, k)...).
		WithSteps(kb.CreationTestSteps(b2, k)...).
		WithSteps(
			helpers.TestStep{
				Name: "Kibana should expose eck info in telemetry data",
				Test: helpers.Eventually(func() error {

					uri := "/api/telemetry/v1/clusters/_stats"
					payload := `{"timeRange":{"min":"0","max":"0"}}`
					body, err := kibana.DoKibanaReq(k, b2, "POST", uri, []byte(payload))
					if err != nil {
						return err
					}

					var stats ClusterStats
					err = json.Unmarshal(body, &stats)
					if err != nil {
						return err
					}

					eck := stats[0].StackStats.Kibana.Plugins.StaticTelemetry.Eck
					if !eck.IsDefined() {
						return fmt.Errorf("eck info not defined properly in telemetry data: %+v", eck)
					}

					return nil
				}),
			},
		).
		WithSteps(es.DeletionTestSteps(b1, k)...).
		WithSteps(kb.DeletionTestSteps(b2, k)...).
		RunSequential(t)
}

// ClusterStats partially models the response from a request to /api/telemetry/v1/clusters/_stats
type ClusterStats []struct {
	StackStats struct {
		Kibana struct {
			Plugins struct {
				StaticTelemetry struct {
					Eck about.OperatorInfo `json:"eck"`
				} `json:"static_telemetry"`
			} `json:"plugins"`
		} `json:"kibana"`
	} `json:"stack_stats"`
}
