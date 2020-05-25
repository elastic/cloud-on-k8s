// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

const httpServiceSuffix = "http"

// Namer is a Namer that is configured with the defaults for resources related to a Kibana resource.
var Namer = common_name.NewNamer("kb")

func HTTPService(kbName string) string {
	return Namer.Suffix(kbName, httpServiceSuffix)
}

func Deployment(kbName string) string {
	return Namer.Suffix(kbName)
}
