// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"reflect"
	"strings"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
	r driver.Interface,
	hasKeystore HasKeystore,
	labels map[string]string,
	namer name.Namer,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	err := watchSecureSettings(r.DynamicWatches(), hasKeystore.SecureSettings(), k8s.ExtractNamespacedName(hasKeystore))
	if err != nil {
		return nil, "", err
	}

	secrets, err := retrieveUserSecrets(r.K8sClient(), r.Recorder(), hasKeystore)
	if err != nil {
		return nil, "", err
	}
	secret, err := reconcileSecureSettings(r.K8sClient(), r.Scheme(), hasKeystore, secrets, namer, labels)
	if err != nil {
		return nil, "", err
	}
	if secret == nil {
		return nil, "", nil
	}

	// build a volume from that secret
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		secret.Name,
		SecureSettingsVolumeName,
		SecureSettingsVolumeMountPath,
	)

	// resource version will be included in pod labels,
	// to recreate pods on any secret change.
	resourceVersion := secret.GetResourceVersion()

	return &secureSettingsVolume, resourceVersion, nil
}

func reconcileSecureSettings(
	c k8s.Client,
	scheme *runtime.Scheme,
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
		err := c.Delete(&expected)
		if apierrors.IsNotFound(err) {
			// swallow not found errors
			return nil, nil
		}
		return nil, err
	}

	reconciled := corev1.Secret{}
	return &reconciled, reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme,
		Owner:      hasKeystore,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
		},
	})
}

func retrieveUserSecrets(c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore) ([]corev1.Secret, error) {
	userSecrets := make([]corev1.Secret, 0, len(hasKeystore.SecureSettings()))
	for _, userSecretsRef := range hasKeystore.SecureSettings() {
		// retrieve the secret referenced by the user in the same namespace
		userSecret, exists, err := retrieveUserSecret(c, recorder, hasKeystore, userSecretsRef)
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

func retrieveUserSecret(c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore, secretSrc commonv1beta1.SecretSource) (*corev1.Secret, bool, error) {
	namespace := hasKeystore.GetNamespace()
	secretName := secretSrc.SecretName

	var userSecret corev1.Secret
	err := c.Get(types.NamespacedName{Namespace: namespace, Name: secretName}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		msg := "Secure settings secret not found"
		log.Info(msg, "namespace", namespace, "secret_name", secretName)
		recorder.Event(hasKeystore, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+secretName)
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	// If no entries, return the whole user secret
	if secretSrc.Entries == nil {
		return &userSecret, true, nil
	}

	if len(secretSrc.Entries) == 0 {
		return nil, false, fmt.Errorf("set is empty in secure settings secret %s", secretName)
	}

	// Else if entries is defined, return only a subset of the user secret
	projectionSecret := corev1.Secret{
		ObjectMeta: userSecret.ObjectMeta,
		Data:       map[string][]byte{},
	}
	for _, entry := range secretSrc.Entries {
		if entry.Key == "" {
			return nil, false, fmt.Errorf("key is empty in secure settings secret %s", secretName)
		}

		newKey := entry.Path
		if newKey == "" {
			newKey = entry.Key
		}

		value, ok := userSecret.Data[entry.Key]
		if !ok {
			return nil, false, fmt.Errorf("key %s not found in secure settings secret %s", entry.Key, secretName)
		}

		projectionSecret.Data[newKey] = value
	}

	return &projectionSecret, true, nil
}

func secureSettingsSecretName(namer name.Namer, hasKeystore HasKeystore) string {
	return namer.Suffix(hasKeystore.GetName(), secureSettingsSecretSuffix)
}

// secureSettingsWatchName returns the watch name according to the deployment name.
// It is unique per APM or Kibana deployment.
func secureSettingsWatchName(namespacedName types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-secure-settings", namespacedName.Namespace, namespacedName.Name)
}

// watchSecureSettings registers a watch for the given secure settings.
//
// Only one watch per cluster is registered:
// - if it already exists with a different secret, it is replaced to watch the new secret.
// - if the given user secret is nil, the watch is removed.
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef []commonv1beta1.SecretSource, nn types.NamespacedName) error {
	watchName := secureSettingsWatchName(nn)
	if secureSettingsRef == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	userSecretNsns := make([]types.NamespacedName, 0, len(secureSettingsRef))
	for _, secretRef := range secureSettingsRef {
		userSecretNsns = append(userSecretNsns, types.NamespacedName{
			Namespace: nn.Namespace,
			Name:      secretRef.SecretName,
		})
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name:    watchName,
		Watched: userSecretNsns,
		Watcher: nn,
	})
}

// Finalizer removes any dynamic watches on external user created secret.
func Finalizer(namespacedName types.NamespacedName, watched watches.DynamicWatches, kind string) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "finalizer." + strings.ToLower(kind) + ".k8s.elastic.co/secure-settings-secret",
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(secureSettingsWatchName(namespacedName))
			return nil
		},
	}
}
