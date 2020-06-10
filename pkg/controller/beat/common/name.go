// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

// namer is a Namer that is configured with the defaults for resources related to a Beat resource.
var namer = common_name.NewNamer("beat")

func ConfigSecretName(typeName, name string) string {
	return namer.Suffix(name, typeName, "config")
}

func Name(name, typeName string) string {
	return namer.Suffix(name, typeName)
}
