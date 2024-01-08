// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pkg/errors"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/telemetry"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func MakeTelemetryRequest(kbBuilder Builder, k *test.K8sClient) (StackStats, error) {
	kbVersion := version.MustParse(kbBuilder.Kibana.Spec.Version)
	apiVersion, payload := apiVersionAndTelemetryRequestBody(kbVersion)
	uri := telemetryAPIURI(kbVersion, apiVersion)
	password, err := k.GetElasticPassword(kbBuilder.ElasticsearchRef().NamespacedName())
	if err != nil {
		return StackStats{}, err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return StackStats{}, err
	}

	// a few extra headers are required by Kibana for this internal API
	extraHeaders := http.Header{}
	if version.WithoutPre(kbVersion).GTE(version.From(8, 10, 0)) {
		extraHeaders.Add("elastic-api-version", "2")
		extraHeaders.Add("kbn-xsrf", "reporting")
		extraHeaders.Add("x-elastic-internal-origin", "eck-e2e-tests")
	}

	// this call may fail (status 500) if the .security-7 index is not fully initialized yet,
	// in which case we'll just retry that test step
	bytes, _, err := DoRequest(k, kbBuilder.Kibana, password, "POST", uri, payloadBytes, extraHeaders)
	if err != nil {
		return StackStats{}, err
	}
	return unmarshalTelemetryResponse(bytes, kbVersion)
}

func telemetryAPIURI(kbVersion version.Version, apiVersion string) string {
	uri := fmt.Sprintf("/api/telemetry/%s/clusters/_stats", apiVersion)
	if version.WithoutPre(kbVersion).GTE(version.From(8, 10, 0)) {
		uri = "/internal/telemetry/clusters/_stats"
	}
	return uri
}

func apiVersionAndTelemetryRequestBody(kbVersion version.Version) (string, telemetryRequest) {
	apiVersion := "v1"
	payload := telemetryRequest{
		TimeRange: &timeRange{},
	}
	if kbVersion.GTE(version.From(7, 2, 0)) {
		apiVersion = "v2"
		payload.Unencrypted = true
	}
	if kbVersion.GTE(version.From(7, 11, 0)) {
		payload.TimeRange = nil // removed in 7.11
	}
	return apiVersion, payload
}

type timeRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// telemetryRequest is the request body for v1/v2 Kibana telemetry requests
type telemetryRequest struct {
	TimeRange   *timeRange `json:"timeRange,omitempty"`
	Unencrypted bool       `json:"unencrypted,omitempty"`
}

func unmarshalTelemetryResponse(bytes []byte, kbVersion version.Version) (StackStats, error) {
	noStatsErr := errors.New("cluster stats is empty")

	// telemetry response changed as of 7.16.0 to the following json key path
	// .[0].stats.stack_stats.kibana.plugins.static_telemetry.eck
	if kbVersion.GTE(version.MinFor(7, 16, 0)) {
		var stats v3Stats
		if err := json.Unmarshal(bytes, &stats); err != nil {
			return StackStats{}, err
		}
		if len(stats) == 0 {
			return StackStats{}, noStatsErr
		}
		return stats[0].Stats.StackStats, nil
	}
	// pre 7.16.0 json key path
	// .[0].stack_stats.kibana.plugins.static_telemetry.eck
	var stats v2Stats
	if err := json.Unmarshal(bytes, &stats); err != nil {
		return StackStats{}, err
	}
	if len(stats) == 0 {
		return StackStats{}, noStatsErr
	}
	return stats[0].StackStats, nil
}

// v3Stats partially models the response from a request to /api/telemetry/v2/clusters/_stats >=7.16.0
type v3Stats []struct {
	Stats struct {
		StackStats StackStats `json:"stack_stats"`
	} `json:"stats"`
}

// v2Stats partially models the response from a request to /api/telemetry/v[1,2]/clusters/_stats <7.16.0
type v2Stats []struct {
	StackStats StackStats `json:"stack_stats"`
}

type StackStats struct {
	Kibana struct {
		Plugins struct {
			StaticTelemetry struct {
				telemetry.ECKTelemetry
			} `json:"static_telemetry"`
		} `json:"plugins"`
	} `json:"kibana"`
}
