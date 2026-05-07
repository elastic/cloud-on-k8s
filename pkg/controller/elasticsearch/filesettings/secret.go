// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"encoding/json"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

const (
	SettingsSecretKey = "settings.json"
)

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
