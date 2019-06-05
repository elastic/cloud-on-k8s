// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/ghodss/yaml"
)

const TelemetryFilename = "telemetry.yml"

type Telemetry struct {
	Eck Eck `json:"eck"`
}

type Eck struct {
	Version string `json:"version"`
}

func getTelemetryYamlBytes() []byte {
	t := Telemetry{
		Eck{
			"0.8.0",
		},
	}
	bytes, _ := yaml.Marshal(t)
	return bytes
}
