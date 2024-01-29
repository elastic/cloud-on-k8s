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

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	commonlabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	SettingsSecretKey = "settings.json"
)

// NewSettingsSecretWithVersion returns a new SettingsSecret for a given Elasticsearch and optionally a current settings
// Secret and a StackConfigPolicy.
// The Settings version is updated using the current timestamp only when the Settings have changed.
// If the new settings from the policy changed compared to the actual from the secret, the settings version is
// updated
func NewSettingsSecretWithVersion(es types.NamespacedName, currentSecret *corev1.Secret, policy *policyv1alpha1.StackConfigPolicy) (corev1.Secret, int64, error) {
	newVersion := time.Now().UnixNano()
	return NewSettingsSecret(newVersion, es, currentSecret, policy)
}

// NewSettingsSecret returns a new SettingsSecret for a given Elasticsearch and StackConfigPolicy.
func NewSettingsSecret(version int64, es types.NamespacedName, currentSecret *corev1.Secret, policy *policyv1alpha1.StackConfigPolicy) (corev1.Secret, int64, error) {
	settings := NewEmptySettings(version)

	// update the settings according to the config policy
	if policy != nil {
		err := settings.updateState(es, *policy)
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
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return corev1.Secret{}, 0, err
	}
	settingsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      esv1.FileSettingsSecretName(es.Name),
			Labels:    eslabel.NewLabels(es),
			Annotations: map[string]string{
				commonannotation.SettingsHashAnnotationName: settings.hash(),
			},
		},
		Data: map[string][]byte{
			SettingsSecretKey: settingsBytes,
		},
	}

	if policy != nil {
		// set this policy as soft owner of this Secret
		SetSoftOwner(settingsSecret, *policy)

		// add the Secure Settings Secret sources to the Settings Secret
		if err := setSecureSettings(settingsSecret, *policy); err != nil {
			return corev1.Secret{}, 0, err
		}
	}

	// Add a label to reset secret on deletion of the stack config policy
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

// SetSoftOwner sets the given StackConfigPolicy as soft owner of the Settings Secret using the "softOwned" labels.
func SetSoftOwner(settingsSecret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) {
	if settingsSecret.Labels == nil {
		settingsSecret.Labels = map[string]string{}
	}
	settingsSecret.Labels[reconciler.SoftOwnerNamespaceLabel] = policy.GetNamespace()
	settingsSecret.Labels[reconciler.SoftOwnerNameLabel] = policy.GetName()
	settingsSecret.Labels[reconciler.SoftOwnerKindLabel] = policyv1alpha1.Kind
}

// setSecureSettings stores the SecureSettings Secret sources referenced in the given StackConfigPolicy in the annotation of the Settings Secret.
func setSecureSettings(settingsSecret *corev1.Secret, policy policyv1alpha1.StackConfigPolicy) error {
	//nolint:staticcheck
	if len(policy.Spec.SecureSettings) == 0 && len(policy.Spec.Elasticsearch.SecureSettings) == 0 {
		return nil
	}

	var secretSources []commonv1.NamespacedSecretSource //nolint:prealloc
	// Common secureSettings field, this is mainly there to maintain backwards compatibility
	//nolint:staticcheck
	for _, src := range policy.Spec.SecureSettings {
		secretSources = append(secretSources, commonv1.NamespacedSecretSource{Namespace: policy.GetNamespace(), SecretName: src.SecretName, Entries: src.Entries})
	}

	// SecureSettings field under Elasticsearch in the StackConfigPolicy
	for _, src := range policy.Spec.Elasticsearch.SecureSettings {
		secretSources = append(secretSources, commonv1.NamespacedSecretSource{Namespace: policy.GetNamespace(), SecretName: src.SecretName, Entries: src.Entries})
	}

	bytes, err := json.Marshal(secretSources)
	if err != nil {
		return err
	}
	settingsSecret.Annotations[commonannotation.SecureSettingsSecretsAnnotationName] = string(bytes)
	return nil
}

// CanBeOwnedBy return true if the Settings Secret can be owned by the given StackConfigPolicy, either because the Secret
// belongs to no one or because it already belongs to the given policy.
func CanBeOwnedBy(settingsSecret corev1.Secret, policy policyv1alpha1.StackConfigPolicy) (reconciler.SoftOwnerRef, bool) {
	currentOwner, referenced := reconciler.SoftOwnerRefFromLabels(settingsSecret.Labels)
	// either there is no soft owner
	if !referenced {
		return reconciler.SoftOwnerRef{}, true
	}
	// or the owner is already the given policy
	canBeOwned := currentOwner.Kind == policyv1alpha1.Kind && currentOwner.Namespace == policy.Namespace && currentOwner.Name == policy.Name
	return currentOwner, canBeOwned
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
