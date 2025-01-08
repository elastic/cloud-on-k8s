// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
)

const (
	httpServiceSuffix      = "http"
	scriptsConfigMapSuffix = "scripts"
	configSecretSuffix     = "config"
)

// KBNamer is a KBNamer that is configured with the defaults for resources related to a Kibana resource.
var KBNamer = common_name.NewNamer("kb")

func HTTPService(kbName string) string {
	return KBNamer.Suffix(kbName, httpServiceSuffix)
}

func Deployment(kbName string) string {
	return KBNamer.Suffix(kbName)
}

// ScriptsConfigMap returns the name of the ConfigMap containing scripts for the given Kibana resource.
func ScriptsConfigMap(kbName string) string {
	return KBNamer.Suffix(kbName, scriptsConfigMapSuffix)
}

// ConfigSecret returns the name of the Secret containing the Kibana configuration for the given Kibana resource.
func ConfigSecret(kb Kibana) string {
	return KBNamer.Suffix(kb.Name, configSecretSuffix)
}
