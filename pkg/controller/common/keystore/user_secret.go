// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"strings"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// secureSettingsVolume creates a volume from the optional user-provided secure settings secret.
//
// Secure settings are provided by the user in the APM or Kibana Spec through a secret reference.
// This secret is mounted into the pods for secure settings to be injected into a keystore.
// The user-provided secret is watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	hasKeystore HasKeystore,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	err := watchSecureSettings(watches, hasKeystore.SecureSettings(), k8s.ExtractNamespacedName(hasKeystore))
	if err != nil {
		return nil, "", err
	}

	if hasKeystore.SecureSettings() == nil {
		// no secure settings secret specified
		return nil, "", nil
	}

	// retrieve the secret referenced by the user in the same namespace
	userSecret, exists, err := retrieveUserSecret(c, recorder, hasKeystore)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		// secret does not exist (yet): no volume to mount
		return nil, "", nil
	}

	// build a volume from that secret
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		userSecret.Name,
		SecureSettingsVolumeName,
		SecureSettingsVolumeMountPath,
	)

	// resource version will be included in pod labels,
	// to recreate pods on any secret change.
	resourceVersion := userSecret.GetResourceVersion()

	return &secureSettingsVolume, resourceVersion, nil
}

func retrieveUserSecret(c k8s.Client, recorder record.EventRecorder, hasKeystore HasKeystore) (*corev1.Secret, bool, error) {
	secretName := hasKeystore.SecureSettings().SecretName
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
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef *commonv1alpha1.SecretRef, nn types.NamespacedName) error {
	watchName := secureSettingsWatchName(nn)
	if secureSettingsRef == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchName,
		Watched: types.NamespacedName{
			Namespace: nn.Namespace,
			Name:      secureSettingsRef.SecretName,
		},
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
