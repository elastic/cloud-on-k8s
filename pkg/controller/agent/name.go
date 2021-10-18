// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"

const httpServiceSuffix = "http"

// Namer is a Namer that is configured with the defaults for resources related to an Agent resource.
var Namer = common_name.NewNamer("agent")

func ConfigSecretName(name string) string {
	return Namer.Suffix(name, "config")
}

func Name(name string) string {
	return Namer.Suffix(name)
}

func HTTPServiceName(name string) string {
	return Namer.Suffix(name, httpServiceSuffix)
}

func EnvVarsSecretName(name string) string {
	return Namer.Suffix(name, "envvars")
}
