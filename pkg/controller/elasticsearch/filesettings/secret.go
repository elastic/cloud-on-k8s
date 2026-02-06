// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	commonlabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	SettingsSecretKey = "settings.json"
)

// NewSettingsSecretWithVersion returns a new SettingsSecret for a given Elasticsearch and optionally a current settings
// Secret and a StackConfigPolicy.
// The Settings version is updated using the current timestamp only when the Settings have changed.
// If the new settings from the policy changed compared to the actual from the secret, the settings version is
// updated
func NewSettingsSecretWithVersion(es types.NamespacedName, currentSecret *corev1.Secret, esConfigPolicy *policyv1alpha1.ElasticsearchConfigPolicySpec, namespacedSecretSources []commonv1.NamespacedSecretSource, meta metadata.Metadata) (corev1.Secret, int64, error) {
	newVersion := time.Now().UnixNano()
	return newSettingsSecret(newVersion, es, currentSecret, esConfigPolicy, namespacedSecretSources, meta)
}

// NewSettingsSecret returns a new SettingsSecret for a given Elasticsearch and StackConfigPolicy.
func newSettingsSecret(version int64, es types.NamespacedName, currentSecret *corev1.Secret, esConfigPolicy *policyv1alpha1.ElasticsearchConfigPolicySpec, namespacedSecretSources []commonv1.NamespacedSecretSource, meta metadata.Metadata) (corev1.Secret, int64, error) {
	settings := NewEmptySettings(version)

	// update the settings according to the config policy
	if esConfigPolicy != nil {
		err := settings.updateState(es, *esConfigPolicy)
		if err != nil {
			return corev1.Secret{}, 0, err
		}
	}

	// do not update version if hash hasn't changed
	if currentSecret != nil && !hasChanged(*currentSecret, settings) {
		currentVersion, err := extractVersion(*currentSecret)
		if err != nil {
			return corev1.Secret{}, 0, err
		}

		version = currentVersion
		settings.Metadata.Version = strconv.FormatInt(currentVersion, 10)
	}

	// prepare the SettingsSecret
	secretMeta := meta.Merge(metadata.Metadata{
		Annotations: map[string]string{
			commonannotation.SettingsHashAnnotationName: settings.hash(),
		},
	})
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return corev1.Secret{}, 0, err
	}
	settingsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   es.Namespace,
			Name:        esv1.FileSettingsSecretName(es.Name),
			Labels:      secretMeta.Labels,
			Annotations: secretMeta.Annotations,
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	// add the Secure Settings Secret sources to the Settings Secret
	if err := setSecureSettings(settingsSecret, namespacedSecretSources); err != nil {
		return corev1.Secret{}, 0, err
	}

	// Add a label to reset secret on deletion of the stack config policy
	if settingsSecret.Labels == nil {
		settingsSecret.Labels = make(map[string]string)
	}
	settingsSecret.Labels[commonlabel.StackConfigPolicyOnDeleteLabelName] = commonlabel.OrphanSecretResetOnPolicyDelete

	return *settingsSecret, version, nil
}

// extractVersion extracts the Settings version from the given settings Secret.
func extractVersion(settingsSecret corev1.Secret) (int64, error) {
	var settings Settings
	err := json.Unmarshal(settingsSecret.Data[SettingsSecretKey], &settings)
	if err != nil {
		return 0, err
	}
	version, err := strconv.ParseInt(settings.Metadata.Version, 10, 64)
	if err != nil {
		return 0, err
	}

	return version, nil
}

// hasChanged compares the hash of the given new Settings Secret with the hash stored in the annotation of the given Settings Secret.
func hasChanged(settingsSecret corev1.Secret, newSettings Settings) bool {
	return settingsSecret.Annotations[commonannotation.SettingsHashAnnotationName] != newSettings.hash()
}

// setSecureSettings stores the SecureSettings Secret sources referenced in the given StackConfigPolicy in the annotation of the Settings Secret.
func setSecureSettings(settingsSecret *corev1.Secret, secretSources []commonv1.NamespacedSecretSource) error {
	if len(secretSources) == 0 {
		return nil
	}

	bytes, err := json.Marshal(secretSources)
	if err != nil {
		return err
	}
	settingsSecret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName] = string(bytes)
	return nil
}

// getSecureSettings returns the SecureSettings Secret sources stores in an annotation of the given file settings Secret.
func getSecureSettings(settingsSecret corev1.Secret) ([]commonv1.NamespacedSecretSource, error) {
	rawString, ok := settingsSecret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName]
	if !ok {
		return []commonv1.NamespacedSecretSource{}, nil
	}
	var secretSources []commonv1.NamespacedSecretSource
	err := json.Unmarshal([]byte(rawString), &secretSources)
	if err != nil {
		return nil, err
	}
	return secretSources, nil
}

// GetSecureSettingsSecretSources gets SecureSettings Secret sources for a given Elastic resource.
func GetSecureSettingsSecretSources(ctx context.Context, c k8s.Client, resource metav1.Object) ([]commonv1.NamespacedSecretSource, error) {
	var secret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: resource.GetNamespace(), Name: esv1.FileSettingsSecretName(resource.GetName())}, &secret)
	if apierrors.IsNotFound(err) {
		return []commonv1.NamespacedSecretSource{}, nil
	}
	if err != nil {
		return nil, err
	}
	secretSources, err := getSecureSettings(secret)
	if err != nil {
		return nil, err
	}
	return secretSources, nil
}
