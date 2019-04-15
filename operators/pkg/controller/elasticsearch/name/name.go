// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"

	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"k8s.io/apimachinery/pkg/util/rand"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("name")
)

const (
	// Whatever the named resource, it must never exceed 63 characters to be used as a label.
	MaxLabelLength = 63
	// Elasticsearch name, used as prefix, is limited to 36 characters,
	MaxElasticsearchNameLength = 36
	// so it leaves 27 characters for a suffix.
	MaxSuffixLength = MaxLabelLength - MaxElasticsearchNameLength
	// podRandomSuffixLength represents the length of the random suffix that is appended in NewPodName.
	podRandomSuffixLength = 10

	podSuffix                  = "-es"
	configSecretSuffix         = "-config"
	secureSettingsSecretSuffix = "-secure-settings"
	certsSecretSuffix          = "-certs"
	serviceSuffix              = "-es"
	discoveryServiceSuffix     = "-es-discovery"
	cASecretSuffix             = "-ca"
	cAPrivateKeySecretSuffix   = "-ca-private-key"
	elasticUserSecretSuffix    = "-elastic-user"
	esRolesUsersSecretSuffix   = "-es-roles-users"
	extraFilesSecretSuffix     = "-extrafiles"
	internalUsersSecretSuffix  = "-internal-users"
	keystoreSecretSuffix       = "-keystore"
)

// Suffix a resource name.
// Panic if the suffix exceeds the limits below.
// Trim the name if it exceeds the limits below.
func suffix(name string, sfx string) string {
	// This should never happen because we control all the suffixes!
	if len(sfx) > MaxSuffixLength {
		panic(fmt.Errorf("suffix should not exceed %d characters: %s", MaxSuffixLength, sfx))
	}
	// This should never happen because the name length should have been validated.
	// Trim the name and log an error as fallback.
	maxPrefixLength := MaxLabelLength - len(sfx)
	if len(name) > maxPrefixLength {
		name = name[:maxPrefixLength]
		log.Error(fmt.Errorf("name should not exceed %d characters: %s", maxPrefixLength, name),
			"Failed to suffix resource")
	}
	return stringsutil.Concat(name, sfx)
}

// NewPodName returns a unique name to be used for the pod name and the
// Elasticsearch cluster node name.
func NewPodName(esName string) string {
	sfx := stringsutil.Concat(
		string(podSuffix),
		"-",
		rand.String(podRandomSuffixLength),
	)
	return suffix(esName, sfx)
}

// NewPVCName returns a unique PVC name given a pod name and a PVC template name.
// Uniqueness is guaranteed by the pod name that contains a random id.
// The PVC template name is trimmed so that the PVC name does not exceed the max
// length for a label.
func NewPVCName(podName string, pvcTemplateName string) string {
	sfx := stringsutil.Concat("-", pvcTemplateName)
	if len(sfx) > MaxSuffixLength {
		sfx = sfx[:MaxSuffixLength]
	}
	return suffix(podName, sfx)
}

func ConfigSecret(podName string) string {
	return suffix(podName, configSecretSuffix)
}

func SecureSettingsSecret(esName string) string {
	return suffix(esName, secureSettingsSecretSuffix)
}

func CertsSecret(podName string) string {
	return suffix(podName, certsSecretSuffix)
}

func Service(esName string) string {
	return suffix(esName, serviceSuffix)
}

func DiscoveryService(esName string) string {
	return suffix(esName, discoveryServiceSuffix)
}

func CASecret(esName string) string {
	return suffix(esName, cASecretSuffix)
}

func CAPrivateKeySecret(esName string) string {
	return suffix(esName, cAPrivateKeySecretSuffix)
}

func ElasticUserSecret(esName string) string {
	return suffix(esName, elasticUserSecretSuffix)
}

func EsRolesUsersSecret(esName string) string {
	return suffix(esName, esRolesUsersSecretSuffix)
}

func ExtraFilesSecret(esName string) string {
	return suffix(esName, extraFilesSecretSuffix)
}

func InternalUsersSecret(esName string) string {
	return suffix(esName, internalUsersSecretSuffix)
}

func KeystoreSecret(esName string) string {
	return suffix(esName, keystoreSecretSuffix)
}
