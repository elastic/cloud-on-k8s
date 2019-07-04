// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	"fmt"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	kbvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

// secureSettingsVolume creates a volume from the optional user-provided secure settings secret.
//
// Secure settings are provided by the user in the Kibana Spec through a secret reference.
// This secret is mounted into Kibana pods for secure settings to be injected into Kibana keystore.
// The user-provided secret is watched to reconcile on any change.
// The user secret resource version is returned along with the volume, so that
// any change in the user secret leads to pod rotation.
func secureSettingsVolume(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	kb v1alpha1.Kibana,
) (*volume.SecretVolume, string, error) {
	// setup (or remove) watches for the user-provided secret to reconcile on any change
	userSecretRef := kb.Spec.SecureSettings
	err := watchSecureSettings(watches, userSecretRef, k8s.ExtractNamespacedName(&kb))
	if err != nil {
		return nil, "", err
	}

	if userSecretRef == nil {
		// no secure settings secret specified
		return nil, "", nil
	}

	// retrieve the secret referenced by the user in the Kibana namespace
	userSecret, exists, err := retrieveUserSecret(c, kb, recorder, kb.Namespace, userSecretRef.SecretName)
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
		kbvolume.SecureSettingsVolumeName,
		kbvolume.SecureSettingsVolumeMountPath,
	)

	// resource version will be included in Kibana pod labels,
	// to recreate pods on any secret change.
	resourceVersion := userSecret.GetResourceVersion()

	return &secureSettingsVolume, resourceVersion, nil
}

func retrieveUserSecret(c k8s.Client, kibana v1alpha1.Kibana, recorder record.EventRecorder, namespace string, name string) (*corev1.Secret, bool, error) {
	userSecret := corev1.Secret{}
	err := c.Get(types.NamespacedName{Namespace: namespace, Name: name}, &userSecret)
	if err != nil && apierrors.IsNotFound(err) {
		msg := "Secure settings secret not found"
		log.Info(msg, "namespace", namespace, "secret_name", name)
		recorder.Event(&kibana, corev1.EventTypeWarning, events.EventReasonUnexpected, msg+": "+name)
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
func watchSecureSettings(watched watches.DynamicWatches, secureSettingsRef *commonv1alpha1.SecretRef, kibana types.NamespacedName) error {
	watchName := secureSettingsWatchName(kibana)
	if secureSettingsRef == nil {
		watched.Secrets.RemoveHandlerForKey(watchName)
		return nil
	}
	return watched.Secrets.AddHandler(watches.NamedWatch{
		Name: watchName,
		Watched: types.NamespacedName{
			Namespace: kibana.Namespace,
			Name:      secureSettingsRef.SecretName,
		},
		Watcher: kibana,
	})
}

// Finalizer removes any dynamic watches on external user created secret.
func Finalizer(kibana types.NamespacedName, watched watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "secure-settings.finalizers.kibana.k8s.elastic.co",
		Execute: func() error {
			watched.Secrets.RemoveHandlerForKey(secureSettingsWatchName(kibana))
			return nil
		},
	}
}
