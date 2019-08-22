// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
)

const (
	// Whatever the named resource, it must never exceed 63 characters to be used as a label.
	MaxLabelLength = 63
	// Elasticsearch name, used as prefix, is limited to 36 characters,
	MaxElasticsearchNameLength = 36
	// this leaves 63 - 36 = 27 characters for a suffix.
	MaxSuffixLength = MaxLabelLength - MaxElasticsearchNameLength

	configSecretSuffix                = "config"
	secureSettingsSecretSuffix        = "secure-settings"
	httpServiceSuffix                 = "http"
	elasticUserSecretSuffix           = "elastic-user"
	xpackFileRealmSecretSuffix        = "xpack-file-realm"
	internalUsersSecretSuffix         = "internal-users"
	unicastHostsConfigMapSuffix       = "unicast-hosts"
	licenseSecretSuffix               = "license"
	defaultPodDisruptionBudget        = "default"
	scriptsConfigMapSuffix            = "scripts"
	transportCertificatesSecretSuffix = "transport-certificates"
)

// ESNamer is a Namer that is configured with the defaults for resources related to an ES cluster.
var ESNamer = name.Namer{
	MaxSuffixLength: MaxSuffixLength,
	DefaultSuffixes: []string{"es"},
}

// StatefulSet returns the name of the StatefulSet corresponding to the given NodeSpec.
func StatefulSet(esName string, nodeSpecName string) string {
	return ESNamer.Suffix(esName, nodeSpecName)
}

func ConfigSecret(ssetName string) string {
	return ESNamer.Suffix(ssetName, configSecretSuffix)
}

func SecureSettingsSecret(esName string) string {
	return ESNamer.Suffix(esName, secureSettingsSecretSuffix)
}

func TransportCertificatesSecret(esName string) string {
	return ESNamer.Suffix(esName, transportCertificatesSecretSuffix)
}

func HTTPService(esName string) string {
	return ESNamer.Suffix(esName, httpServiceSuffix)
}

func ElasticUserSecret(esName string) string {
	return ESNamer.Suffix(esName, elasticUserSecretSuffix)
}

func XPackFileRealmSecret(esName string) string {
	return ESNamer.Suffix(esName, xpackFileRealmSecretSuffix)
}

func InternalUsersSecret(esName string) string {
	return ESNamer.Suffix(esName, internalUsersSecretSuffix)
}

// UnicastHostsConfigMap returns the name of the ConfigMap that holds the list of seed nodes for a given cluster.
func UnicastHostsConfigMap(esName string) string {
	return ESNamer.Suffix(esName, unicastHostsConfigMapSuffix)
}

func ScriptsConfigMap(esName string) string {
	return ESNamer.Suffix(esName, scriptsConfigMapSuffix)
}

func LicenseSecretName(esName string) string {
	return ESNamer.Suffix(esName, licenseSecretSuffix)
}

func DefaultPodDisruptionBudget(esName string) string {
	return ESNamer.Suffix(esName, defaultPodDisruptionBudget)
}
