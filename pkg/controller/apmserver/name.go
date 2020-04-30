// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

const (
	secretTokenSuffix = "token"
	httpServiceSuffix = "http"
	configSuffix      = "config"
	deploymentSuffix  = "server"
)

// Namer configured with the defaults for resources related to an APM resource.
var Namer = common_name.NewNamer("apm")

func SecretToken(apmName string) string {
	return Namer.Suffix(apmName, secretTokenSuffix)
}

func HTTPService(apmName string) string {
	return Namer.Suffix(apmName, httpServiceSuffix)
}

func Deployment(apmName string) string {
	return Namer.Suffix(apmName, deploymentSuffix)
}

func Config(apmName string) string {
	return Namer.Suffix(apmName, configSuffix)
}
