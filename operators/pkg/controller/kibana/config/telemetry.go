// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/info"
	"github.com/ghodss/yaml"
)

const telemetryFilename = "telemetry.yml"

// getTelemetryYamlBytes returns the YAML bytes for the information on ECK.
func getTelemetryYamlBytes() []byte {
	bytes, _ := yaml.Marshal(struct {
		ECK info.Info `json:"eck"`
	}{
		info.Get(),
	})
	return bytes
}
