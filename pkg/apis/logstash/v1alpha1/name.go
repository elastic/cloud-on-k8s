// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
)

const (
	apiServiceSuffix = "api"
	configSuffix     = "config"
	pipelineSuffix   = "pipeline"
)

// Namer is a Namer that is configured with the defaults for resources related to a Logstash resource.
var Namer = common_name.NewNamer("ls")

// ConfigSecretName returns the name of a secret used to storage Logstash configuration data.
func ConfigSecretName(name string) string {
	return Namer.Suffix(name, configSuffix)
}

// Name returns the name of Logstash.
func Name(name string) string {
	return Namer.Suffix(name)
}

// APIServiceName returns the name of the HTTP service for a given Logstash name.
func APIServiceName(name string) string {
	return Namer.Suffix(name, apiServiceSuffix)
}

func UserServiceName(deployName string, name string) string {
	return Namer.Suffix(deployName, name)
}

func PipelineSecretName(name string) string {
	return Namer.Suffix(name, pipelineSuffix)
}
