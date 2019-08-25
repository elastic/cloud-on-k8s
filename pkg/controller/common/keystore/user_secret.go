// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"reflect"
	"strings"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
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
// The user provicded secrets are then aggregated into a single secret.
// This secret is mounted into the pods for secure settings to be injected into a keystore.
// The user-provided secrets are watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	c k8s.Client,
	scheme *runtime.Scheme,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	hasKeystore HasKeystore,
	labels map[string]string,
	namer name.Namer,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	err := watchSecureSettings(watches, hasKeystore.SecureSettings(), k8s.ExtractNamespacedName(hasKeystore))
	if err != nil {
		return nil, "", err
	}

	secrets, err := retrieveUserSecrets(c, recorder, hasKeystore)
	if err != nil {
		return nil, "", err
	}
	secret, err := reconcileSecureSettings(c, scheme, hasKeystore, secrets, namer, labels)
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
	for _, userSecretRef := range hasKeystore.SecureSettings() {
		// retrieve the secret referenced by the user in the same namespace
		userSecret, exists, err := retrieveUserSecret(c, recorder, hasKeystore, userSecretRef.SecretName)
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

func retrieveUserSecret(c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore, secretName string) (*corev1.Secret, bool, error) {
	namespace := hasKeystore.GetNamespace()

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
	return &userSecret, true, nil
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
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef []commonv1alpha1.SecretRef, nn types.NamespacedName) error {
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
// TODO: Kind of an object can be retrieved programmatically with object.GetObjectKind(), unfortunately it does not seem
//  to be reliable with controller-runtime < v0.2.0-beta.4
func Finalizer(namespacedName types.NamespacedName, watched watches.DynamicWatches, kind string) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "secure-settings.finalizers." + strings.ToLower(kind) + ".k8s.elastic.co",
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(secureSettingsWatchName(namespacedName))
			return nil
		},
	}
}
