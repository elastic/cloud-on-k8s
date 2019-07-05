// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"fmt"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// secureSettingsVolume creates a volume from the optional user-provided secure settings secret.
//
// Secure settings are provided by the user in the Spec through a secret reference.
// This secret is mounted into pods for secure settings to be injected into the keystore.
// The user-provided secret is watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	owner runtime.Object,
	namespacedName types.NamespacedName,
	secureSettingsVolumeName string,
	secureSettingsVolumeMountPath string,
	secureSettingsSecretsRef *commonv1alpha1.SecretRef,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	err := watchSecureSettings(watches, secureSettingsSecretsRef, namespacedName)
	if err != nil {
		return nil, "", err
	}

	if secureSettingsSecretsRef == nil {
		// no secure settings secret specified
		return nil, "", nil
	}

	// retrieve the secret referenced by the user in the namespace of the custom resource owner
	secureSettingsSecret, exists, err := retrieveUserSecret(c, recorder,
		owner,
		namespacedName.Namespace,
		secureSettingsSecretsRef.SecretName,
	)
	if err != nil {
		return nil, "", err
	}
	if !exists {
		// secret does not exist (yet): no volume to mount
		return nil, "", nil
	}

	// build a volume from that secret
	secureSettingsVolume := volume.NewSecretVolumeWithMountPath(
		secureSettingsSecret.Name,
		secureSettingsVolumeName,
		secureSettingsVolumeMountPath,
	)

	// resource version will be included in pod labels,
	// to recreate pods on any secret change.
	resourceVersion := secureSettingsSecret.GetResourceVersion()

	return &secureSettingsVolume, resourceVersion, nil
}

func retrieveUserSecret(
	c k8s.Client,
	recorder record.EventRecorder,
	owner runtime.Object,
	namespace string,
	name string,
) (*corev1.Secret, bool, error) {
	userSecret := corev1.Secret{}
	err := c.Get(types.NamespacedName{Namespace: namespace, Name: name}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		msg := "Secure settings secret not found"
		log.Info(msg, "name", name)
		recorder.Event(owner, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+name)
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	return &userSecret, true, nil
}

// secureSettingsWatchName returns the watch name according to the owner name.
// It is unique per owner.
func secureSettingsWatchName(namespacedName types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-secure-settings", namespacedName.Namespace, namespacedName.Name)
}

// watchSecureSettings registers a watch for the given secure settings.
//
// Only one watch per cluster is registered:
// - if it already exists with a different secret, it is replaced to watch the new secret.
// - if the given user secret is nil, the watch is removed.
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef *commonv1alpha1.SecretRef, namespacedName types.NamespacedName) error {
	watchName := secureSettingsWatchName(namespacedName)
	if secureSettingsRef == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchName,
		Watched: types.NamespacedName{
			Namespace: namespacedName.Namespace,
			Name:      secureSettingsRef.SecretName,
		},
		Watcher: namespacedName,
	})
}

// Finalizer removes any dynamic watches on external user created secret.
func Finalizer(name string, namespacedName types.NamespacedName, watched watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: fmt.Sprintf("secure-settings.finalizers.%s.k8s.elastic.co", name),
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(secureSettingsWatchName(namespacedName))
			return nil
		},
	}
}
