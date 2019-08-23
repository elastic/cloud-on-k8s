// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/ghodss/yaml"
)

const telemetryFilename = "telemetry.yml"

type ECK struct {
	ECK about.OperatorInfo `json:"eck"`
}

// getTelemetryYamlBytes returns the YAML bytes for the information on ECK.
func getTelemetryYamlBytes(operatorInfo about.OperatorInfo) ([]byte, error) {
	return yaml.Marshal(ECK{operatorInfo})
}
