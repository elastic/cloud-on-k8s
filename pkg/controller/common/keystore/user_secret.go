// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"context"
	"fmt"

	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const secureSettingsSecretSuffix = "secure-settings"

// secureSettingsVolume creates a volume from the optional user-provided secure settings secrets.
//
// Secure settings are provided by the user in the resource Spec through secret references.
// The user provided secrets are then aggregated into a single secret.
// This secret is mounted into the pods for secure settings to be injected into a keystore.
// The user-provided secrets are watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	ctx context.Context,
	r driver.Interface,
	hasKeystore HasKeystore,
	labels map[string]string,
	namer name.Namer,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	watcher := k8s.ExtractNamespacedName(hasKeystore)

	// user-provided Secrets referenced in the resource
	secretSources := WatchedSecretNames(hasKeystore)
	// user-provided Secrets referenced in a StackConfigPolicy that configures the resource
	policySecretSources, err := stackconfigpolicy.GetSecureSettingsSecretSourcesForResources(ctx, r.K8sClient(), hasKeystore, hasKeystore.GetObjectKind().GroupVersionKind().Kind)
	if err != nil {
		return nil, "", pkgerrors.Wrap(err, "fail to get secure settings secret sources")
	}
	secretSources = append(secretSources, policySecretSources...)

	if err := watches.WatchUserProvidedNamespacedSecrets(
		watcher,
		r.DynamicWatches(),
		SecureSettingsWatchName(watcher),
		secretSources,
	); err != nil {
		return nil, "", err
	}

	userSecrets, err := retrieveUserSecrets(ctx, r.K8sClient(), r.Recorder(), hasKeystore, secretSources)
	if err != nil {
		return nil, "", err
	}

	secureSettingsSecret, err := reconcileSecureSettings(ctx, r.K8sClient(), hasKeystore, userSecrets, namer, labels)
	if err != nil {
		return nil, "", err
	}
	if secureSettingsSecret == nil {
		return nil, "", nil
	}

	// build a volume from that secret
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		secureSettingsSecret.Name,
		SecureSettingsVolumeName,
		SecureSettingsVolumeMountPath,
	)

	// secret data hash will be included in pod labels to recreate pods on any secret change
	secureSettingsSecretHash := hash.HashObject(secureSettingsSecret.Data)

	return &secureSettingsVolume, secureSettingsSecretHash, nil
}

func reconcileSecureSettings(
	ctx context.Context,
	c k8s.Client,
	hasKeystore HasKeystore,
	userSecrets []corev1.Secret,
	namer name.Namer,
	labels map[string]string) (*corev1.Secret, error) {
	aggregatedData := map[string][]byte{}

	for _, s := range userSecrets {
		for k, v := range s.Data {
			aggregatedData[k] = v
		}
	}

	// reconcile our managed secret with the user-provided secret content
	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secureSettingsSecretName(namer, hasKeystore),
			Namespace: hasKeystore.GetNamespace(),
			Labels:    labels,
		},
		Data: aggregatedData,
	}
	if len(aggregatedData) == 0 {
		// no secure settings specified, delete any existing operator-managed settings secret
		err := k8s.DeleteSecretIfExists(ctx, c, k8s.ExtractNamespacedName(&expected))
		return nil, err
	}

	secret, err := reconciler.ReconcileSecret(ctx, c, expected, hasKeystore)
	if err != nil {
		return nil, err
	}
	return &secret, nil
}

func retrieveUserSecrets(ctx context.Context, c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore, userSecretSources []commonv1.NamespacedSecretSource) ([]corev1.Secret, error) {
	userSecrets := make([]corev1.Secret, 0, len(userSecretSources))
	for _, userSecretsRef := range userSecretSources {
		// retrieve the secret referenced by the user in the same namespace
		userSecret, exists, err := retrieveUserSecret(ctx, c, recorder, hasKeystore, userSecretsRef)
		if err != nil {
			return nil, err
		}
		if !exists {
			// a secret does not exist (yet)
			continue
		}
		userSecrets = append(userSecrets, *userSecret)
	}
	return userSecrets, nil
}

func retrieveUserSecret(ctx context.Context, c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore, secretSrc commonv1.NamespacedSecretSource) (*corev1.Secret, bool, error) {
	secretNamespace := secretSrc.Namespace
	secretName := secretSrc.SecretName
	var userSecret corev1.Secret
	err := c.Get(context.Background(), types.NamespacedName{Namespace: secretNamespace, Name: secretName}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		msg := "Secure settings secret not found"
		ulog.FromContext(ctx).Info(msg, "namespace", secretNamespace, "secret_name", secretName)
		recorder.Event(hasKeystore, corev1.EventTypeWarning, events.EventReasonUnexpected, fmt.Sprintf("%s: %s/%s", msg, secretNamespace, secretName))
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	// If no entries, return the whole user secret
	if secretSrc.Entries == nil {
		return &userSecret, true, nil
	}

	if len(secretSrc.Entries) == 0 {
		return nil, false, pkgerrors.Errorf("set is empty in secure settings secret %s", secretName)
	}

	// Else if entries is defined, return only a subset of the user secret
	projectionSecret := corev1.Secret{
		ObjectMeta: userSecret.ObjectMeta,
		Data:       map[string][]byte{},
	}
	for _, entry := range secretSrc.Entries {
		if entry.Key == "" {
			return nil, false, pkgerrors.Errorf("key is empty in secure settings secret %s", secretName)
		}

		newKey := entry.Path
		if newKey == "" {
			newKey = entry.Key
		}

		value, ok := userSecret.Data[entry.Key]
		if !ok {
			return nil, false, pkgerrors.Errorf("key %s not found in secure settings secret %s", entry.Key, secretName)
		}

		projectionSecret.Data[newKey] = value
	}

	return &projectionSecret, true, nil
}

func secureSettingsSecretName(namer name.Namer, hasKeystore HasKeystore) string {
	return namer.Suffix(hasKeystore.GetName(), secureSettingsSecretSuffix)
}

// SecureSettingsWatchName returns the watch name according to the deployment name.
// It is unique per APM or Kibana deployment.
func SecureSettingsWatchName(namespacedName types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-secure-settings", namespacedName.Namespace, namespacedName.Name)
}
