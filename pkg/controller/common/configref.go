// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"fmt"

	"github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ConfigRefWatchName returns the name of the watch registered on the secret referenced in `configRef`.
func ConfigRefWatchName(resource types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-configref", resource.Namespace, resource.Name)
}

// ParseConfigRef retrieves the content of a secret referenced in `configRef`, sets up dynamic watches for that secret,
// and parses the secret content into a CanonicalConfig.
func ParseConfigRef(
	driver driver.Interface,
	resource runtime.Object, // eg. Beat, EnterpriseSearch
	configRef *commonv1.ConfigSource,
	secretKey string, // retrieve config data from that entry in the secret
) (*settings.CanonicalConfig, error) {
	parsed, err := ParseConfigRefToConfig(driver, resource, configRef, secretKey, ConfigRefWatchName, settings.Options)
	if err != nil {
		return nil, err
	}
	return (*settings.CanonicalConfig)(parsed), nil
}

// ParseConfigRefToConfig retrieves the content of a secret referenced in `configRef`, sets up dynamic watches for that secret,
// and parses the secret content into ucfg.Config.
func ParseConfigRefToConfig(
	driver driver.Interface,
	resource runtime.Object, // eg. Beat, EnterpriseSearch
	configRef *commonv1.ConfigSource,
	secretKey string, // retrieve config data from that entry in the secret
	configRefWatchName func(types.NamespacedName) string,
	configOptions []ucfg.Option,
) (*ucfg.Config, error) {
	var ref *commonv1.ConfigMapOrSecretSource
	if configRef != nil {
		ref = &commonv1.ConfigMapOrSecretSource{SecretRef: configRef.SecretRef}
	}
	return ParseConfigMapOrSecretRefToConfig(driver, resource, ref, secretKey, "configRef", configRefWatchName, nil, configOptions)
}

// ParseConfigMapOrSecretRefToConfig retrieves config from either a Secret or ConfigMap referenced in ref,
// manages dynamic watches for both sources, and parses the content into ucfg.Config.
// refName is used in error/event messages (e.g. "configRef", "pipelinesRef").
// configMapWatchName may be nil when ConfigMap sources are not supported by the caller.
func ParseConfigMapOrSecretRefToConfig(
	driver driver.Interface,
	resource runtime.Object,
	ref *commonv1.ConfigMapOrSecretSource,
	key string,
	refName string,
	secretWatchName func(types.NamespacedName) string,
	configMapWatchName func(types.NamespacedName) string,
	configOptions []ucfg.Option,
) (*ucfg.Config, error) {
	resourceMeta, err := meta.Accessor(resource)
	if err != nil {
		return nil, err
	}
	namespace := resourceMeta.GetNamespace()
	resourceNsn := types.NamespacedName{Namespace: namespace, Name: resourceMeta.GetName()}

	// Derive sources once; these slices drive both watch management and fetch dispatch.
	var secretNames []string
	var configMapNsns []types.NamespacedName
	if ref != nil {
		if ref.SecretName != "" {
			secretNames = []string{ref.SecretName}
		}
		if ref.ConfigMapName != "" {
			configMapNsns = []types.NamespacedName{{Namespace: namespace, Name: ref.ConfigMapName}}
		}
	}

	// Watches are registered before fetching so that a missing object is still watched
	// and triggers reconciliation once it is created.
	if secretWatchName != nil {
		if err := watches.WatchUserProvidedSecrets(resourceNsn, driver.DynamicWatches(), secretWatchName(resourceNsn), secretNames); err != nil {
			return nil, err
		}
	}
	if configMapWatchName != nil {
		if err := watches.WatchUserProvidedConfigMaps(resourceNsn, driver.DynamicWatches(), configMapWatchName(resourceNsn), configMapNsns); err != nil {
			return nil, err
		}
	}

	var data []byte
	var parseEventAction, source string

	switch {
	case len(secretNames) > 0:
		var secret corev1.Secret
		if err := driver.K8sClient().Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: ref.SecretName}, &secret); err != nil {
			return nil, err
		}
		d, exists := secret.Data[key]
		if !exists {
			msg := fmt.Sprintf("unable to retrieve %s secret %s/%s: missing key %s", refName, namespace, ref.SecretName, key)
			k8s.EmitEvent(driver.Recorder(), resource, corev1.EventTypeWarning, events.EventReasonUnexpected, events.EventActionGetSecret, msg)
			return nil, errors.New(msg)
		}
		data, parseEventAction, source = d, events.EventActionParseSecret, fmt.Sprintf("%s secret %s/%s", refName, namespace, ref.SecretName)
	case len(configMapNsns) > 0:
		var cm corev1.ConfigMap
		if err := driver.K8sClient().Get(context.Background(), configMapNsns[0], &cm); err != nil {
			return nil, err
		}
		d, exists := cm.Data[key]
		if !exists {
			msg := fmt.Sprintf("unable to retrieve %s configmap %s/%s: missing key %s", refName, namespace, ref.ConfigMapName, key)
			k8s.EmitEvent(driver.Recorder(), resource, corev1.EventTypeWarning, events.EventReasonUnexpected, events.EventActionGetConfigMap, msg)
			return nil, errors.New(msg)
		}
		data, parseEventAction, source = []byte(d), events.EventActionParseConfigMap, fmt.Sprintf("%s configmap %s/%s", refName, namespace, ref.ConfigMapName)
	default:
		return nil, nil
	}

	parsed, err := uyaml.NewConfig(data, configOptions...)
	if err != nil {
		msg := fmt.Sprintf("unable to parse %s in %s", key, source)
		k8s.EmitEvent(driver.Recorder(), resource, corev1.EventTypeWarning, events.EventReasonUnexpected, parseEventAction, msg)
		return nil, errors.Wrap(err, msg)
	}
	return parsed, nil
}
