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
)

// EntNamer is a Namer that is configured with the defaults for resources related to an EnterpriseSearch resource.
var EntNamer = common_name.NewNamer("ent")

func HTTPService(entName string) string {
	return EntNamer.Suffix(entName, httpServiceSuffix)
}

func Deployment(entName string) string {
	return EntNamer.Suffix(entName)
}

func Config(entName string) string {
	return EntNamer.Suffix(entName, configSuffix)
}
