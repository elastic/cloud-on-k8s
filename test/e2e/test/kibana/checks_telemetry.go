// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func MakeTelemetryRequest(kbBuilder Builder, k *test.K8sClient) ([]byte, error) {
	kbVersion := version.MustParse(kbBuilder.Kibana.Spec.Version)
	apiVersion, payload := apiVersionAndTelemetryRequestBody(kbVersion)
	uri := fmt.Sprintf("/api/telemetry/%s/clusters/_stats", apiVersion)
	password, err := k.GetElasticPassword(kbBuilder.ElasticsearchRef().NamespacedName())
	if err != nil {
		return nil, err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	// this call may fail (status 500) if the .security-7 index is not fully initialized yet,
	// in which case we'll just retry that test step
	return DoRequest(k, kbBuilder.Kibana, password, "POST", uri, payloadBytes)

}

func apiVersionAndTelemetryRequestBody(kbVersion version.Version) (string, telemetryRequest) {
	apiVersion := "v1"
	payload := telemetryRequest{
		TimeRange: &timeRange{},
	}
	if kbVersion.IsSameOrAfter(version.From(7, 2, 0)) {
		apiVersion = "v2"
		payload.Unencrypted = true
	}
	if kbVersion.IsSameOrAfter(version.From(7, 11, 0)) {
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
