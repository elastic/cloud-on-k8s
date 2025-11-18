// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"

const (
	httpServiceSuffix = "http"
	configSuffix      = "config"
)

func HTTPServiceName(eprName string) string {
	return eprv1alpha1.Namer.Suffix(eprName, httpServiceSuffix)
}

func DeploymentName(eprName string) string {
	return eprv1alpha1.Namer.Suffix(eprName)
}

func ConfigName(eprName string) string {
	return eprv1alpha1.Namer.Suffix(eprName, configSuffix)
}
