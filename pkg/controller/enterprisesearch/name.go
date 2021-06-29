// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"

const (
	httpServiceSuffix = "http"
	configSuffix      = "config"
)

func HTTPServiceName(entName string) string {
	return entv1.Namer.Suffix(entName, httpServiceSuffix)
}

func DeploymentName(entName string) string {
	return entv1.Namer.Suffix(entName)
}

func ConfigName(entName string) string {
	return entv1.Namer.Suffix(entName, configSuffix)
}
