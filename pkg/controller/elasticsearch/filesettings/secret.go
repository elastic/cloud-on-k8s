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

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

const (
	SecureSettingsSecretsAnnotationName = "policy.k8s.elastic.co/secure-settings-secrets" //nolint:gosec
	settingsHashAnnotationName          = "policy.k8s.elastic.co/settings-hash"
	settingsSecretKey                   = "settings.json"
)

// NewSettingsSecret returns a new SettingsSecret for a given Elasticsearch and StackConfigPolicy.
func NewSettingsSecret(version int64, current *SettingsSecret, es types.NamespacedName, policy *policyv1alpha1.StackConfigPolicy) (SettingsSecret, error) {
	version, settings := NewSettings(version)

	// update the settings according the config policy
	if policy != nil {
		err := settings.updateState(es, *policy)
		if err != nil {
			return SettingsSecret{}, err
		}
	}

	// do not increment version if hash hasn't changed
	if current != nil && !current.hasChanged(settings) {
		version = current.Version
		settings.Metadata.Version = current.Settings.Metadata.Version
	}

	// prepare the SettingsSecret
	settingsBytes, err := json.Marshal(settings)
	if err != nil {
		return SettingsSecret{}, err
	}
	settingsSecret := SettingsSecret{
		Secret: corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: es.Namespace,
				Name:      esv1.FileSettingsSecretName(es.Name),
				Labels:    eslabel.NewLabels(es),
				Annotations: map[string]string{
					settingsHashAnnotationName: settings.hash(),
				},
			},
			Data: map[string][]byte{
				settingsSecretKey: settingsBytes,
			},
		},
		Settings: settings,
		Version:  version,
	}

	return settingsSecret, nil
}

// NewSettingsSecretFromSecret returns a new Settings Secret from a given Secret.
func NewSettingsSecretFromSecret(secret corev1.Secret) (SettingsSecret, error) {
	var settings Settings
	err := json.Unmarshal(secret.Data[settingsSecretKey], &settings)
	if err != nil {
		return SettingsSecret{}, err
	}
	version, err := strconv.ParseInt(settings.Metadata.Version, 10, 64)
	if err != nil {
		return SettingsSecret{}, err
	}

	return SettingsSecret{
		Secret:   secret,
		Settings: settings,
		Version:  version,
	}, nil
}

// SettingsSecret wraps a Secret used to store File based Settings and the corresponding Settings and their version stored in it.
type SettingsSecret struct {
	corev1.Secret
	Settings Settings
	Version  int64
}

// hasChanged compares the hash of the given settings with the hash stored in the annotation of the Settings Secret.
func (s SettingsSecret) hasChanged(newSettings Settings) bool {
	return s.Annotations[settingsHashAnnotationName] != newSettings.hash()
}

// CanBeOwnedBy return true if the Settings Secret can be owned by the given StackConfigPolicy, either because the Secret
// belongs to no one or because it already belongs to the given policy.
func (s SettingsSecret) CanBeOwnedBy(policy policyv1alpha1.StackConfigPolicy) (reconciler.SoftOwnerRef, bool) {
	currentOwner, referenced := reconciler.SoftOwnerRefFromLabels(s.Labels)
	// either there is no soft owner
	if !referenced {
		return reconciler.SoftOwnerRef{}, true
	}
	// or the owner is already the given policy
	canBeOwned := currentOwner.Kind == policy.Kind && currentOwner.Namespace == policy.Namespace && currentOwner.Name == policy.Name
	return currentOwner, canBeOwned
}

// SetSoftOwner sets the given StackConfigPolicy as soft owner of the Settings Secret using the "softOwned" labels.
func (s *SettingsSecret) SetSoftOwner(policy policyv1alpha1.StackConfigPolicy) {
	if s.Labels == nil {
		s.Labels = map[string]string{}
	}
	s.Labels[reconciler.SoftOwnerNamespaceLabel] = policy.GetNamespace()
	s.Labels[reconciler.SoftOwnerNameLabel] = policy.GetName()
	s.Labels[reconciler.SoftOwnerKindLabel] = policy.GetObjectKind().GroupVersionKind().Kind
}

// SetSecureSettings stores the SecureSettings Secret sources referenced in the given StackConfigPolicy in the annotation of the Settings Secret.
func (s *SettingsSecret) SetSecureSettings(policy policyv1alpha1.StackConfigPolicy) error {
	if len(policy.Spec.SecureSettings) == 0 {
		return nil
	}

	secretSources := make([]commonv1.NamespacedSecretSource, len(policy.Spec.SecureSettings))
	for i, src := range policy.Spec.SecureSettings {
		secretSources[i] = commonv1.NamespacedSecretSource{Namespace: policy.GetNamespace(), SecretName: src.SecretName, Entries: src.Entries}
	}

	bytes, err := json.Marshal(secretSources)
	if err != nil {
		return err
	}
	s.Annotations[SecureSettingsSecretsAnnotationName] = string(bytes)
	return nil
}

// getSecureSettings returns the SecureSettings Secret sources stores in an annotation of the Secret.
func (s *SettingsSecret) getSecureSettings() ([]commonv1.NamespacedSecretSource, error) {
	rawString, ok := s.Annotations[SecureSettingsSecretsAnnotationName]
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
	settingsSecret, err := NewSettingsSecretFromSecret(secret)
	if err != nil {
		return nil, err
	}
	secretSources, err := settingsSecret.getSecureSettings()
	if err != nil {
		return nil, err
	}
	return secretSources, nil
}
