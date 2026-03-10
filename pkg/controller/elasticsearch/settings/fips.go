// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	keystorePasswordEnvVar       = "KEYSTORE_PASSWORD"
	keystorePassphraseFileEnvVar = "ES_KEYSTORE_PASSPHRASE_FILE" //nolint:gosec // Environment variable name, not a secret value.
)

// IsFIPSEnabled returns true when the given config contains
// xpack.security.fips_mode.enabled: true.
func IsFIPSEnabled(cfg *common.CanonicalConfig) bool {
	if cfg == nil {
		return false
	}
	val, err := cfg.String("xpack.security.fips_mode.enabled")
	if err != nil {
		return false
	}
	return val == "true"
}

// AnyNodeSetFIPSEnabled returns true if any NodeSet's user-supplied config or
// the StackConfigPolicy config has xpack.security.fips_mode.enabled: true.
// It inspects the raw config layers directly rather than performing a full
// merged config computation.
func AnyNodeSetFIPSEnabled(nodeSets []esv1.NodeSet, policyConfig *common.CanonicalConfig) (bool, error) {
	if IsFIPSEnabled(policyConfig) {
		return true, nil
	}
	for i := range nodeSets {
		if nodeSets[i].Config == nil {
			continue
		}
		cfg, err := common.NewCanonicalConfigFrom(nodeSets[i].Config.Data)
		if err != nil {
			return false, fmt.Errorf("parsing config for NodeSet %s: %w", nodeSets[i].Name, err)
		}
		if IsFIPSEnabled(cfg) {
			return true, nil
		}
	}
	return false, nil
}

// HasUserProvidedKeystorePassword returns true if the user has set
// KEYSTORE_PASSWORD or ES_KEYSTORE_PASSPHRASE_FILE on the Elasticsearch
// container in their pod template. It checks both explicit env entries and
// envFrom sources by resolving referenced ConfigMaps and Secrets.
func HasUserProvidedKeystorePassword(ctx context.Context, c k8s.Client, namespace string, podTemplate corev1.PodTemplateSpec) (bool, error) {
	for _, container := range podTemplate.Spec.Containers {
		if container.Name != esv1.ElasticsearchContainerName {
			continue
		}
		for _, env := range container.Env {
			if env.Name == keystorePasswordEnvVar || env.Name == keystorePassphraseFileEnvVar {
				return true, nil
			}
		}
		found, err := envFromContainsKeystorePassword(ctx, c, namespace, container.EnvFrom)
		if err != nil {
			return false, fmt.Errorf("while checking envFrom for keystore password vars: %w", err)
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

// AnyNodeSetHasUserProvidedKeystorePassword returns true if any NodeSet pod
// template provides a keystore password override through env or envFrom.
func AnyNodeSetHasUserProvidedKeystorePassword(
	ctx context.Context,
	c k8s.Client,
	namespace string,
	nodeSets []esv1.NodeSet,
) (bool, error) {
	for _, nodeSet := range nodeSets {
		hasOverride, err := HasUserProvidedKeystorePassword(ctx, c, namespace, nodeSet.PodTemplate)
		if err != nil {
			return false, err
		}
		if hasOverride {
			return true, nil
		}
	}
	return false, nil
}

// envFromContainsKeystorePassword resolves the ConfigMaps and Secrets
// referenced by the given envFrom entries and returns true if any of them
// would inject KEYSTORE_PASSWORD or ES_KEYSTORE_PASSPHRASE_FILE.
func envFromContainsKeystorePassword(ctx context.Context, c k8s.Client, namespace string, sources []corev1.EnvFromSource) (bool, error) {
	for _, src := range sources {
		if src.ConfigMapRef != nil {
			var cm corev1.ConfigMap
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: src.ConfigMapRef.Name}, &cm); err != nil {
				if apierrors.IsNotFound(err) && src.ConfigMapRef.Optional != nil && *src.ConfigMapRef.Optional {
					continue
				}
				return false, err
			}
			if envFromKeyMatches(src.Prefix, cm.Data) {
				return true, nil
			}
		}
		if src.SecretRef != nil {
			var secret corev1.Secret
			if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: src.SecretRef.Name}, &secret); err != nil {
				if apierrors.IsNotFound(err) && src.SecretRef.Optional != nil && *src.SecretRef.Optional {
					continue
				}
				return false, err
			}
			// secret.Data is map[string][]byte; extract keys only to match envFromKeyMatches signature.
			stringData := make(map[string]string, len(secret.Data))
			for k := range secret.Data {
				stringData[k] = ""
			}
			if envFromKeyMatches(src.Prefix, stringData) {
				return true, nil
			}
		}
	}
	return false, nil
}

// envFromKeyMatches returns true if any key in data, when prefixed with the
// given envFrom prefix, matches a keystore password env var name.
func envFromKeyMatches(prefix string, data map[string]string) bool {
	for key := range data {
		name := prefix + key
		if name == keystorePasswordEnvVar || name == keystorePassphraseFileEnvVar {
			return true
		}
	}
	return false
}
