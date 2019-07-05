// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/name"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	// Whatever the named resource, it must never exceed 63 characters to be used as a label.
	MaxLabelLength = 63
	// Elasticsearch name, used as prefix, is limited to 36 characters,
	MaxElasticsearchNameLength = 36
	// this leaves 63 - 36 = 27 characters for a suffix.
	MaxSuffixLength = MaxLabelLength - MaxElasticsearchNameLength
	// podRandomSuffixLength represents the length of the random suffix that is appended in NewPodName.
	podRandomSuffixLength = 10

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

var esNoDefaultSuffixesNamer = ESNamer.WithDefaultSuffixes()

// NewPodName returns a unique name to be used for the pod name and the
// Elasticsearch cluster node name.
// The generated pod name follows the pattern "{esName}-es-[{nodeSpec.Name}-]{random suffix}".
func NewPodName(esName string, nodeSpec v1alpha1.NodeSpec) string {
	var sfx strings.Builder

	// it's safe to ignore the result here as strings.Builder cannot error on sfx.WriteString
	if nodeSpec.Name != "" {
		sfx.WriteString(nodeSpec.Name) // #nosec G104
		sfx.WriteString("-")           // #nosec G104
	}

	sfx.WriteString(rand.String(podRandomSuffixLength)) // #nosec G104

	return ESNamer.Suffix(esName, sfx.String())
}

// Basename returns the base name (without the random suffix) for the provided pod.
// E.g: A pod named foo-bar-baz-{suffix} has a basename of "foo-bar-baz".
func Basename(podName string) string {
	idx := strings.LastIndex(podName, "-")
	if idx == -1 {
		// no segments in the provided pod name, so return the full pod name
		return podName
	}
	return podName[0:idx]
}

// StatefulSet returns the name of the StatefulSet corresponding to the given NodeSpec.
func StatefulSet(esName string, nodeSpecName string) string {
	return ESNamer.Suffix(esName, nodeSpecName)
}

// NewPVCName returns a unique PVC name given a pod name and a PVC template name.
// Uniqueness is guaranteed by the pod name that contains a random id.
// The PVC template name is trimmed so that the PVC name does not exceed the max
// length for a label.
func NewPVCName(podName string, pvcTemplateName string) string {
	if len(pvcTemplateName) > MaxSuffixLength {
		pvcTemplateName = pvcTemplateName[:MaxSuffixLength-1]
	}
	return esNoDefaultSuffixesNamer.Suffix(podName, pvcTemplateName)
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
