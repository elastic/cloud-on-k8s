// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// secureSettingsVolume creates a volume from the optional user-provided secure settings secret.
//
// Secure settings are provided by the user in the APM Spec through a secret reference.
// This secret is mounted into APM pods for secure settings to be injected into APM keystore.
// The user-provided secret is watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	apm v1alpha1.ApmServer,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	userSecretRef := apm.Spec.SecureSettings
	err := watchSecureSettings(watches, userSecretRef, k8s.ExtractNamespacedName(&apm))
	if err != nil {
		return nil, "", err
	}

	if userSecretRef == nil {
		// no secure settings secret specified
		return nil, "", nil
	}

	// retrieve the secret referenced by the user in the APM namespace
	userSecret, exists, err := retrieveUserSecret(c, apm, recorder, apm.Namespace, userSecretRef.SecretName)
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

	// resource version will be included in APM pod labels,
	// to recreate pods on any secret change.
	resourceVersion := userSecret.GetResourceVersion()

	return &secureSettingsVolume, resourceVersion, nil
}

func retrieveUserSecret(c k8s.Client, apm v1alpha1.ApmServer, recorder record.EventRecorder, namespace string, name string) (*corev1.Secret, bool, error) {
	userSecret := corev1.Secret{}
	err := c.Get(types.NamespacedName{Namespace: namespace, Name: name}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		msg := "Secure settings secret not found"
		log.Info(msg, "name", name)
		recorder.Event(&apm, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+name)
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	return &userSecret, true, nil
}

// secureSettingsWatchName returns the watch name according to the Kibana deployment name.
// It is unique per Kibana deployment.
func secureSettingsWatchName(kibana types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-secure-settings", kibana.Namespace, kibana.Name)
}

// watchSecureSettings registers a watch for the given secure settings.
//
// Only one watch per cluster is registered:
// - if it already exists with a different secret, it is replaced to watch the new secret.
// - if the given user secret is nil, the watch is removed.
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef *commonv1alpha1.SecretRef, apm types.NamespacedName) error {
	watchName := secureSettingsWatchName(apm)
	if secureSettingsRef == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchName,
		Watched: types.NamespacedName{
			Namespace: apm.Namespace,
			Name:      secureSettingsRef.SecretName,
		},
		Watcher: apm,
	})
}

// Finalizer removes any dynamic watches on external user created secret.
func Finalizer(apm types.NamespacedName, watched watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "secure-settings.finalizers.kibana.k8s.elastic.co",
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(secureSettingsWatchName(apm))
			return nil
		},
	}
}
