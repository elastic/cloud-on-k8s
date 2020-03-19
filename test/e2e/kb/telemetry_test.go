// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"encoding/json"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
)

func TestTelemetry(t *testing.T) {
	name := "test-telemetry"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources)
	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	stepsFn := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			{
				Name: "Kibana should expose eck info in telemetry data",
				Test: func(t *testing.T) {
					uri := "/api/telemetry/v1/clusters/_stats"
					payload := `{"timeRange":{"min":"0","max":"0"}}`
					body, err := kibana.DoKibanaReq(k, kbBuilder.Kibana, "POST", uri, []byte(payload))
					require.NoError(t, err)

					var stats ClusterStats
					err = json.Unmarshal(body, &stats)
					require.NoError(t, err)
					eck := stats[0].StackStats.Kibana.Plugins.StaticTelemetry.Eck
					if !eck.IsDefined() {
						t.Errorf("eck info not defined properly in telemetry data: %+v", eck)
					}
				},
			},
		}
	}

	test.Sequence(nil, stepsFn, esBuilder, kbBuilder)

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
