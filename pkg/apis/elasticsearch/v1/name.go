// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	utilvalidation "k8s.io/apimachinery/pkg/util/validation"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
)

const (
	configSecretSuffix                           = "config"
	secureSettingsSecretSuffix                   = "secure-settings"
	fileSettingsSecretSuffix                     = "file-settings"
	policyEsConfigSecretSuffix                   = "policy-config" //nolint:gosec
	httpServiceSuffix                            = "http"
	internalHTTPServiceSuffix                    = "internal-http"
	remoteClusterServiceSuffix                   = "remote-cluster"
	transportServiceSuffix                       = "transport"
	elasticUserSecretSuffix                      = "elastic-user"
	internalUsersSecretSuffix                    = "internal-users"
	unicastHostsConfigMapSuffix                  = "unicast-hosts"
	licenseSecretSuffix                          = "license"
	defaultPodDisruptionBudget                   = "default"
	scriptsConfigMapSuffix                       = "scripts"
	legacyTransportCertsSecretSuffix             = "transport-certificates"
	statefulSetTransportCertificatesSecretSuffix = "transport-certs"

	// calling this secret "xpack-file-realm" is conceptually wrong since it also holds the file-based roles which
	// are not part of the file realm - let's still keep this legacy name for convenience
	rolesAndFileRealmSecretSuffix = "xpack-file-realm" //nolint:gosec

	// remoteCaNameSuffix is a suffix for the secret that contains the concatenation of all the remote CAs
	remoteCaNameSuffix = "remote-ca"

	// remoteAPIKeysNameSuffix is a suffix for the secret that contains the API keys for the remote clusters.
	remoteAPIKeysNameSuffix = "remote-api-keys"

	controllerRevisionHashLen = 10
)

var (
	// ESNamer is a Namer that is configured with the defaults for resources related to an ES cluster.
	ESNamer = common_name.NewNamer("es")

	suffixes = []string{
		configSecretSuffix,
		secureSettingsSecretSuffix,
		httpServiceSuffix,
		elasticUserSecretSuffix,
		rolesAndFileRealmSecretSuffix,
		internalUsersSecretSuffix,
		unicastHostsConfigMapSuffix,
		licenseSecretSuffix,
		defaultPodDisruptionBudget,
		scriptsConfigMapSuffix,
		statefulSetTransportCertificatesSecretSuffix,
		remoteCaNameSuffix,
		remoteAPIKeysNameSuffix,
	}
)

// ValidateNames checks the validity of resource names that will be generated by the given Elasticsearch object.
func ValidateNames(es Elasticsearch) error {
	if len(es.Name) > common_name.MaxResourceNameLength {
		return errors.Errorf("name exceeds maximum allowed length of %d", common_name.MaxResourceNameLength)
	}
	nodeSetNames := map[string]struct{}{}
	// validate ssets
	for _, nodeSet := range es.Spec.NodeSets {
		if _, ok := nodeSetNames[nodeSet.Name]; ok {
			return errors.Errorf("duplicated nodeSet name: '%s'", nodeSet.Name)
		}
		nodeSetNames[nodeSet.Name] = struct{}{}

		if errs := apimachineryvalidation.NameIsDNSSubdomain(nodeSet.Name, false); len(errs) > 0 {
			return errors.Errorf("invalid nodeSet name '%s': [%s]", nodeSet.Name, strings.Join(errs, ","))
		}

		ssetName, err := ESNamer.SafeSuffix(es.Name, nodeSet.Name)
		if err != nil {
			return errors.Wrapf(err, "error generating StatefulSet name for nodeSet: '%s'", nodeSet.Name)
		}

		// length of the ordinal suffix that will be added to the pods of this sset (dash + ordinal)
		podOrdinalSuffixLen := len(strconv.FormatInt(int64(nodeSet.Count), 10)) + 1
		// there should be enough space for the ordinal suffix and the controller revision hash
		if utilvalidation.LabelValueMaxLength-len(ssetName) < podOrdinalSuffixLen+controllerRevisionHashLen {
			return errors.Errorf("generated StatefulSet name '%s' exceeds allowed length of %d",
				ssetName,
				utilvalidation.LabelValueMaxLength-podOrdinalSuffixLen-controllerRevisionHashLen)
		}
	}

	// validate other suffixes
	for _, suffix := range suffixes {
		if _, err := ESNamer.SafeSuffix(es.Name, suffix); err != nil {
			return err
		}
	}

	return nil
}

// StatefulSet returns the name of the StatefulSet corresponding to the given NodeSet.
func StatefulSet(esName string, nodeSetName string) string {
	return ESNamer.Suffix(esName, nodeSetName)
}

func ConfigSecret(ssetName string) string {
	return ESNamer.Suffix(ssetName, configSecretSuffix)
}

func SecureSettingsSecret(esName string) string {
	return ESNamer.Suffix(esName, secureSettingsSecretSuffix)
}

func StatefulSetTransportCertificatesSecret(ssetName string) string {
	return ESNamer.Suffix(ssetName, statefulSetTransportCertificatesSecretSuffix)
}

// LegacyTransportCertsSecretSuffix returns the former name of the Secret which used to contain the transport certificates.
// This function only exists to let the controller delete that Secret.
func LegacyTransportCertsSecretSuffix(esName string) string {
	return ESNamer.Suffix(esName, legacyTransportCertsSecretSuffix)
}

func TransportService(esName string) string {
	return ESNamer.Suffix(esName, transportServiceSuffix)
}

func InternalHTTPService(esName string) string {
	return ESNamer.Suffix(esName, internalHTTPServiceSuffix)
}

func RemoteClusterService(esName string) string {
	return ESNamer.Suffix(esName, remoteClusterServiceSuffix)
}

func HTTPService(esName string) string {
	return ESNamer.Suffix(esName, httpServiceSuffix)
}

func ElasticUserSecret(esName string) string {
	return ESNamer.Suffix(esName, elasticUserSecretSuffix)
}

func RolesAndFileRealmSecret(esName string) string {
	return ESNamer.Suffix(esName, rolesAndFileRealmSecretSuffix)
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

func RemoteCaSecretName(esName string) string {
	return ESNamer.Suffix(esName, remoteCaNameSuffix)
}

func RemoteAPIKeysSecretName(esName string) string {
	return ESNamer.Suffix(esName, remoteAPIKeysNameSuffix)
}

func FileSettingsSecretName(esName string) string {
	return ESNamer.Suffix(esName, fileSettingsSecretSuffix)
}

func StackConfigElasticsearchConfigSecretName(esName string) string {
	return ESNamer.Suffix(esName, policyEsConfigSecretSuffix)
}

// StackConfigAdditionalSecretName returns the name of the stack config policy Secret suffixed with a hash to prevent conflicts.
// This also helps keep the secret name size to within kubernetes name limits even if the secret name created by the user is long.
func StackConfigAdditionalSecretName(esName string, secretName string) string {
	secretNameHash := hash.HashObject(secretName)
	return ESNamer.Suffix(esName, "scp", secretNameHash)
}
