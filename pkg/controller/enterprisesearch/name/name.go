// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

const (
	httpServiceSuffix = "http"
	configSuffix      = "config"
	deploymentSuffix  = "server"
)

// EntSearchNamer is a Namer that is configured with the defaults for resources related to an EnterpriseSearch resource.
// TODO: "entsearch" looks better but we reach the 28 chars suffix limit for certs secret :(
var EntSearchNamer = common_name.NewNamer("ents")

func HTTPService(entsName string) string {
	return EntSearchNamer.Suffix(entsName, httpServiceSuffix)
}

func Deployment(entsName string) string {
	return EntSearchNamer.Suffix(entsName, deploymentSuffix)
}

func Config(entsName string) string {
	return EntSearchNamer.Suffix(entsName, configSuffix)
}
