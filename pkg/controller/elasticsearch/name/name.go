// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	common_name "github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	apimachineryvalidation "k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
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

var (
	// ESNamer is a Namer that is configured with the defaults for resources related to an ES cluster.
	ESNamer = common_name.NewNamer("es")

	suffixes = []string{
		configSecretSuffix,
		secureSettingsSecretSuffix,
		httpServiceSuffix,
		elasticUserSecretSuffix,
		xpackFileRealmSecretSuffix,
		internalUsersSecretSuffix,
		unicastHostsConfigMapSuffix,
		licenseSecretSuffix,
		defaultPodDisruptionBudget,
		scriptsConfigMapSuffix,
		transportCertificatesSecretSuffix,
	}
)

// Validate checks the validity of resource names that will be generated by the given Elasticsearch object.
func Validate(es v1alpha1.Elasticsearch) error {
	esName := k8s.ExtractNamespacedName(&es).Name
	if len(esName) > common_name.MaxResourceNameLength {
		return fmt.Errorf("name exceeds maximum allowed length of %d", common_name.MaxResourceNameLength)
	}

	if errs := apimachineryvalidation.NameIsDNSSubdomain(esName, false); len(errs) > 0 {
		return fmt.Errorf("invalid Elasticsearch name: '%s'", esName)
	}

	// validate ssets
	for _, nodeSpec := range es.Spec.Nodes {
		ssetName, err := ESNamer.SafeSuffix(esName, nodeSpec.Name)
		if err != nil {
			return errors.Wrapf(err, "error generating StatefulSet name for nodeSpec: '%s'", nodeSpec.Name)
		}

		// length of the ordinal suffix that will be added to the pods of this sset
		podOrdinalSuffixLen := len(strconv.FormatInt(int64(nodeSpec.NodeCount), 10))
		// there should be enough space for the ordinal suffix
		if validation.DNS1123SubdomainMaxLength-len(ssetName) < podOrdinalSuffixLen {
			return fmt.Errorf("generated StatefulSet name '%s' exceeds allowed length of %d",
				ssetName,
				validation.DNS1123SubdomainMaxLength-podOrdinalSuffixLen)
		}
	}

	// validate other suffixes
	// it would be better to use the actual naming functions here but they panic on error
	for _, suffix := range suffixes {
		if _, err := ESNamer.SafeSuffix(esName, suffix); err != nil {
			return err
		}
	}

	return nil
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
