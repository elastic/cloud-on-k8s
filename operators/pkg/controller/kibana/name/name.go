// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	common_name "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/name"
)

const (
	// Whatever the named resource, it must never exceed 63 characters to be used as a label.
	MaxLabelLength = 63
	// Elasticsearch name, used as prefix, is limited to 36 characters,
	MaxElasticsearchNameLength = 36
	// this leaves 63 - 36 = 27 characters for a suffix.
	MaxSuffixLength = MaxLabelLength - MaxElasticsearchNameLength

	httpServiceSuffix          = "http"
	secureSettingsSecretSuffix = "secure-settings"
)

// KBNamer is a Namer that is configured with the defaults for resources related to a Kibana resource.
var KBNamer = common_name.Namer{
	MaxSuffixLength: MaxSuffixLength,
	DefaultSuffixes: []string{"kb"},
}

func HTTPService(kbName string) string {
	return KBNamer.Suffix(kbName, httpServiceSuffix)
}

func Deployment(kbName string) string {
	return KBNamer.Suffix(kbName)
}

func SecureSettingsSecret(kbName string) string {
	return KBNamer.Suffix(kbName, secureSettingsSecretSuffix)
}
