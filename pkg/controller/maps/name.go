// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"

const (
	httpServiceSuffix = "http"
	configSuffix      = "config"
)

// EMSNamer is a Namer that is configured with the defaults for resources related to an Elastic Maps Server resource.
var EMSNamer = name.NewNamer("ems")

func HTTPService(emsName string) string {
	return EMSNamer.Suffix(emsName, httpServiceSuffix)
}

func Deployment(emsName string) string {
	return EMSNamer.Suffix(emsName)
}

func Config(emasName string) string {
	return EMSNamer.Suffix(emasName, configSuffix)
}
