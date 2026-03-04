// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

const (
	keystorePasswordEnvVar       = "KEYSTORE_PASSWORD"
	keystorePassphraseFileEnvVar = "ES_KEYSTORE_PASSPHRASE_FILE" //nolint:gosec // Environment variable name, not a secret value.
)

// IsFIPSEnabled returns true when the merged Elasticsearch config contains
// xpack.security.fips_mode.enabled: true.
func IsFIPSEnabled(cfg CanonicalConfig) bool {
	val, err := cfg.String("xpack.security.fips_mode.enabled")
	if err != nil {
		return false
	}
	return val == "true"
}

// AnyNodeSetFIPSEnabled returns true if any of the provided per-NodeSet
// configs has xpack.security.fips_mode.enabled: true.
func AnyNodeSetFIPSEnabled(configs []CanonicalConfig) bool {
	return slices.ContainsFunc(configs, IsFIPSEnabled)
}

// HasUserProvidedKeystorePassword returns true if the user has set
// KEYSTORE_PASSWORD or ES_KEYSTORE_PASSPHRASE_FILE on the Elasticsearch
// container in their pod template.
func HasUserProvidedKeystorePassword(podTemplate corev1.PodTemplateSpec) bool {
	for _, c := range podTemplate.Spec.Containers {
		if c.Name != esv1.ElasticsearchContainerName {
			continue
		}
		for _, env := range c.Env {
			if env.Name == keystorePasswordEnvVar || env.Name == keystorePassphraseFileEnvVar {
				return true
			}
		}
	}
	return false
}
