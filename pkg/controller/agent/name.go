// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"

const httpServiceSuffix = "http"

// namer is a Namer that is configured with the defaults for resources related to an Agent resource.
var namer = common_name.NewNamer("agent")

func ConfigSecretName(name string) string {
	return namer.Suffix(name, "config")
}

func Name(name string) string {
	return namer.Suffix(name)
}

func HttpServiceName(name string) string {
	return namer.Suffix(name, httpServiceSuffix)
}
